package source

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A permalink pins a SHA, which --branch cannot take. The ref is ignored, so
// the clone succeeds whatever it says.
func TestFetchIgnoresRef(t *testing.T) {
	repo := initRepo(t)

	dir, cleanup, err := Fetch(t.Context(), Source{
		CloneURL:   repo,
		IgnoredRef: "c0f91d41c28b02c8142a3161fbbb089bab35f051",
		Subpath:    "skills/foo",
		Display:    repo,
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	defer cleanup()

	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		t.Errorf("subpath not checked out: %v", err)
	}
}

// A source beginning with a dash must land in git's URL position, not its option
// position, where --upload-pack names a command to run.
func TestFetchDoesNotLetASourceBecomeAnOption(t *testing.T) {
	requireGit(t)

	marker := filepath.Join(t.TempDir(), "executed")
	_, cleanup, err := Fetch(t.Context(), Source{
		CloneURL: "--upload-pack=touch " + marker,
		Display:  "hostile",
	})
	if cleanup != nil {
		cleanup()
	}
	if err == nil {
		t.Fatal("Fetch succeeded, want a clone failure")
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatal("git ran the injected command")
	}
	// It failed as a repository name rather than as a rejected flag.
	if !strings.Contains(err.Error(), "upload-pack") {
		t.Errorf("unexpected error: %v", err)
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	requireGit(t)

	repo := t.TempDir()
	skillDir := filepath.Join(repo, "skills", "foo")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: foo\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "-A"},
		{"-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-qm", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return repo
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}
