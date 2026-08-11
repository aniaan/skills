package source

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseHub(t *testing.T) {
	tests := []struct {
		label   string
		in      string
		ref     string
		display string // defaults to in
		wantErr bool
	}{
		{label: "owner and slug", in: "https://hub.example.com/owner/note-taking"},
		{label: "trailing slash", in: "https://hub.example.com/owner/note-taking/"},
		{label: "slug only", in: "https://hub.example.com/note-taking"},
		{label: "http", in: "http://hub.internal/o/s"},
		// Reported, then dropped: a registry source always takes the latest.
		// The query does not survive into Display, which errors print.
		{label: "version query", in: "https://hub.internal/o/s?version=1.2.3", ref: "1.2.3", display: "https://hub.internal/o/s"},
		{label: "tag query", in: "https://hub.internal/o/s?tag=latest", ref: "latest", display: "https://hub.internal/o/s"},
		{label: "no path", in: "https://hub.internal", wantErr: true},
		{label: "no host", in: "https:///note-taking", wantErr: true},
		{label: "credentials", in: "https://someone:hunter2@hub.example.com/o/s", wantErr: true},
		{label: "not a url", in: "owner/repo", wantErr: true},
		{label: "ssh url", in: "git@github.com:owner/repo.git", wantErr: true},
		{label: "empty", in: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			got, err := Parse(hubPrefix + tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parsing %q as a registry succeeded, want an error", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("parsing %q as a registry: %v", tt.in, err)
			}
			if got.IgnoredRef != tt.ref {
				t.Errorf("IgnoredRef = %q, want %q", got.IgnoredRef, tt.ref)
			}
			display := tt.display
			if display == "" {
				display = tt.in
			}
			// Display echoes the source as typed, prefix and all.
			if want := hubPrefix + display; got.Display != want {
				t.Errorf("Display = %q, want %q", got.Display, want)
			}
			if got.IsLocal() || got.CloneURL != "" {
				t.Errorf("parsed as %+v, want neither a local nor a git source", got)
			}
			if got.fetch == nil {
				t.Fatal("no fetcher")
			}
		})
	}
}

// The web page is /{owner}/{slug}; the API is indexed by slug alone.
func TestParseHubRequestsSlug(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != downloadPath {
			t.Errorf("requested %s, want %s", r.URL.Path, downloadPath)
		}
		got = r.URL.Query().Get("slug")
		w.Write(zipped(t, map[string]string{"SKILL.md": "---\nname: s\n---\n"}))
	}))
	t.Cleanup(srv.Close)

	for _, tt := range []struct{ path, want string }{
		{"/owner/note-taking", "note-taking"},
		{"/owner/note-taking/", "note-taking"},
		{"/note-taking", "note-taking"},
		{"/o/s?version=1.2.3", "s"},
	} {
		t.Run(tt.path, func(t *testing.T) {
			got = ""
			src, err := Parse(hubPrefix + srv.URL + tt.path)
			if err != nil {
				t.Fatal(err)
			}
			if err := src.fetch(t.Context(), t.TempDir()); err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("requested slug %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFetchHub(t *testing.T) {
	srv := registry(t, zipped(t, map[string]string{
		"SKILL.md":        "---\nname: note-taking\n---\n",
		"references/a.md": "a",
		"scripts/one.sh":  "#!/bin/sh\n",
		"_meta.json":      `{"slug":"note-taking"}`,
	}))

	src, err := Parse(hubPrefix + srv.URL + "/owner/note-taking")
	if err != nil {
		t.Fatal(err)
	}
	dir, cleanup, err := Fetch(t.Context(), src)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	defer cleanup()

	for _, name := range []string{"SKILL.md", "references/a.md", "scripts/one.sh", "_meta.json"} {
		info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			t.Errorf("%s not extracted: %v", name, err)
			continue
		}
		// 0644 is requested, not guaranteed — the umask still narrows it. What
		// must hold is that trusting the archive's 0666 did not hand anything
		// out executable or writable beyond the owner.
		if perm := info.Mode().Perm(); perm&0o133 != 0 {
			t.Errorf("%s is %04o, want no execute or group/other write bit", name, perm)
		}
	}
}

// Bytes are not the only cost an archive can impose.
func TestFetchHubStopsACrowdedArchive(t *testing.T) {
	files := make(map[string]string, memberLimit+1)
	for i := range memberLimit + 1 {
		files[fmt.Sprintf("f%d.md", i)] = ""
	}

	src, err := Parse(hubPrefix + registry(t, zipped(t, files)).URL + "/o/s")
	if err != nil {
		t.Fatal(err)
	}
	_, cleanup, err := Fetch(t.Context(), src)
	if cleanup != nil {
		cleanup()
	}
	if err == nil {
		t.Fatal("Fetch succeeded, want a rejected archive")
	}
	if !strings.Contains(err.Error(), "members") {
		t.Errorf("unexpected error: %v", err)
	}
}

// A registry may hand off to a CDN, but not back down to plain HTTP.
func TestClientRefusesADowngrade(t *testing.T) {
	secure := httptest.NewRequest(http.MethodGet, "https://hub.example.com"+downloadPath, nil)
	plain := httptest.NewRequest(http.MethodGet, "http://cdn.example.com/x.zip", nil)

	if err := client.CheckRedirect(plain, []*http.Request{secure}); err == nil {
		t.Error("https to http redirect accepted")
	}
	if err := client.CheckRedirect(secure, []*http.Request{secure}); err != nil {
		t.Errorf("https to https redirect refused: %v", err)
	}
}

// A registry chooses these bytes, so a member reaching outside must fail rather
// than land beside the temp directory.
func TestFetchHubRejectsEscapingMembers(t *testing.T) {
	for _, name := range []string{"../escaped.md", "../../escaped.md", "/tmp/escaped.md"} {
		t.Run(name, func(t *testing.T) {
			srv := registry(t, zipped(t, map[string]string{name: "x"}))

			src, err := Parse(hubPrefix + srv.URL + "/o/s")
			if err != nil {
				t.Fatal(err)
			}
			dir, cleanup, err := Fetch(t.Context(), src)
			if cleanup != nil {
				cleanup()
			}
			if err == nil {
				t.Fatalf("Fetch succeeded into %s, want a rejected member", dir)
			}
			if !strings.Contains(err.Error(), "extract") {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// Every rejection prints the URL back, so no rejection may repeat a secret in
// it. Credentials are refused rather than dropped, since the request would go
// out unauthenticated either way.
func TestParseHubNeverQuotesASecret(t *testing.T) {
	for _, in := range []string{
		"https://someone:hunter2@hub.example.com/o/s",
		"https://hub.example.com?token=hunter2",
		"https:///x?token=hunter2",
		"ftp://someone:hunter2@hub.example.com/o/s",
	} {
		t.Run(in, func(t *testing.T) {
			_, err := Parse(hubPrefix + in)
			if err == nil {
				t.Fatal("ParseHub succeeded, want an error")
			}
			if strings.Contains(err.Error(), "hunter2") {
				t.Errorf("error quotes the secret: %v", err)
			}
		})
	}
}

// The member cap counts names; nesting multiplies them into directories.
func TestFetchHubStopsADeepArchive(t *testing.T) {
	deep := strings.Repeat("a/", memberDepth) + "f.md"

	src, err := Parse(hubPrefix + registry(t, zipped(t, map[string]string{deep: "x"})).URL + "/o/s")
	if err != nil {
		t.Fatal(err)
	}
	_, cleanup, err := Fetch(t.Context(), src)
	if cleanup != nil {
		cleanup()
	}
	if err == nil {
		t.Fatal("Fetch succeeded, want a rejected archive")
	}
	if !strings.Contains(err.Error(), "nested deeper") {
		t.Errorf("unexpected error: %v", err)
	}
}

// A registry chooses the compression ratio too.
func TestFetchHubStopsAnExpandingArchive(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("bomb.md")
	if err != nil {
		t.Fatal(err)
	}
	chunk := make([]byte, 1<<20)
	for range (archiveLimit >> 20) + 1 {
		if _, err := w.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	src, err := Parse(hubPrefix + registry(t, buf.Bytes()).URL + "/o/s")
	if err != nil {
		t.Fatal(err)
	}
	dir, cleanup, err := Fetch(t.Context(), src)
	if cleanup != nil {
		cleanup()
	}
	if err == nil {
		t.Fatalf("archive extracted into %s", dir)
	}
	if !strings.Contains(err.Error(), "expands past") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFetchHubReportsRegistryError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not found", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	src, err := Parse(hubPrefix + srv.URL + "/o/missing")
	if err != nil {
		t.Fatal(err)
	}
	_, cleanup, err := Fetch(t.Context(), src)
	if cleanup != nil {
		cleanup()
	}
	if err == nil {
		t.Fatal("Fetch succeeded, want a download failure")
	}
	if !strings.Contains(err.Error(), "Not found") {
		t.Errorf("error does not quote the registry: %v", err)
	}
}

func registry(t *testing.T, archive []byte) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != downloadPath {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Write(archive)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// zipped builds an archive the way a registry does: no Unix permissions, and no
// entries for the directories its members imply.
func zipped(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
