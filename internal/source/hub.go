package source

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

// A registry is any host serving the ClawHub HTTP API: clawhub.ai, a
// self-hosted deployment, or another product shipping the same routes.
const (
	downloadPath    = "/api/v1/download"
	archiveLimit    = 32 << 20
	memberLimit     = 4096
	memberDepth     = 64
	downloadTimeout = 2 * time.Minute
)

// A registry may hand off to a CDN, but a redirect must not walk back down to
// plain HTTP. Replacing CheckRedirect also replaces the default hop limit.
var client = &http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("stopped after %d redirects", len(via))
		}
		if via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
			return fmt.Errorf("refusing redirect to %s", req.URL.Scheme)
		}
		return nil
	},
}

// ParseHub reads a URL copied from a registry's skill page, which is
// /{owner}/{slug}. The API is indexed by slug alone, so the owner is display
// only. A version in the query is reported and ignored, like a git ref.
func ParseHub(arg string) (Source, error) {
	u, err := url.Parse(arg)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return Source{}, fmt.Errorf("unrecognized registry URL %q\n"+
			"expected https://host/owner/skill-name", redact(arg))
	}

	// Refused rather than dropped: the request would go out unauthenticated
	// while the password stayed in the messages the user sees.
	if u.User != nil {
		return Source{}, fmt.Errorf("credentials in a registry URL are not supported: %s", redact(arg))
	}

	segments := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
	if len(segments) == 0 {
		return Source{}, fmt.Errorf("no skill named in %q\n"+
			"expected https://host/owner/skill-name", redact(arg))
	}
	slug := segments[len(segments)-1]

	version := u.Query().Get("version")
	if version == "" {
		version = u.Query().Get("tag")
	}

	// The API lives at the origin; a registry behind a path prefix is not one
	// this recognizes.
	base := (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()

	return Source{
		IgnoredRef: version,
		Display:    redact(arg),
		fetch:      downloadInto(base, slug),
	}, nil
}

// redact strips userinfo and query from a URL before it reaches Display or an
// error, both of which are printed.
func redact(raw string) string {
	// Scheme, not host: a hostless URL still carries a query.
	if u, err := url.Parse(raw); err == nil && u.Scheme != "" {
		u.User = nil
		u.RawQuery = ""
		u.Fragment = ""
		return u.String()
	}
	// Unparseable, so cut anything shaped like userinfo by hand.
	if i := strings.Index(raw, "://"); i >= 0 {
		if j := strings.Index(raw[i+3:], "@"); j >= 0 {
			return raw[:i+3] + raw[i+3+j+1:]
		}
	}
	return raw
}

func downloadInto(base, slug string) fetcher {
	return func(ctx context.Context, dest string) error {
		ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
		defer cancel()

		endpoint := base + downloadPath + "?" + url.Values{"slug": {slug}}.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}

		resp, err := client.Do(req)
		if err != nil {
			// *url.Error names the address it failed on, which after a redirect
			// can be a signed one. The cause alone says enough.
			var urlErr *url.Error
			if errors.As(err, &urlErr) {
				err = urlErr.Err
			}
			return fmt.Errorf("download %s: %w", endpoint, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("download %s: %s%s", endpoint, resp.Status, reason(resp.Body))
		}

		archive, err := io.ReadAll(io.LimitReader(resp.Body, archiveLimit+1))
		if err != nil {
			return fmt.Errorf("download %s: %w", endpoint, err)
		}
		if len(archive) > archiveLimit {
			return fmt.Errorf("download %s: larger than %d bytes", endpoint, archiveLimit)
		}
		return unzip(archive, dest)
	}
}

// reason quotes a failed response; registries answer in plain text.
func reason(body io.Reader) string {
	text, err := io.ReadAll(io.LimitReader(body, 200))
	if err != nil {
		return ""
	}
	if text = bytes.TrimSpace(text); len(text) == 0 {
		return ""
	}
	return ": " + string(text)
}

// unzip writes through an *os.Root, so a member named ../x fails instead of
// escaping.
func unzip(archive []byte, dest string) error {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return fmt.Errorf("read archive: %w", err)
	}

	// Bytes are not the only cost: a small archive can still name enough members
	// to exhaust inodes and time.
	if len(zr.File) > memberLimit {
		return fmt.Errorf("archive holds more than %d members", memberLimit)
	}

	root, err := os.OpenRoot(dest)
	if err != nil {
		return err
	}
	defer root.Close()

	budget := int64(archiveLimit)
	for _, f := range zr.File {
		name := path.Clean(f.Name)

		// The member cap counts names, not the directories MkdirAll would
		// synthesize behind them.
		if strings.Count(name, "/") >= memberDepth {
			return fmt.Errorf("extract %s: nested deeper than %d directories", f.Name, memberDepth)
		}

		switch {
		case f.FileInfo().IsDir():
			if err := root.MkdirAll(name, 0o755); err != nil {
				return fmt.Errorf("extract %s: %w", f.Name, err)
			}

		case f.Mode().IsRegular():
			if dir := path.Dir(name); dir != "." {
				if err := root.MkdirAll(dir, 0o755); err != nil {
					return fmt.Errorf("extract %s: %w", f.Name, err)
				}
			}
			written, err := writeMember(root, f, name, budget)
			if err != nil {
				return fmt.Errorf("extract %s: %w", f.Name, err)
			}
			if budget -= written; budget < 0 {
				return fmt.Errorf("archive expands past %d bytes", archiveLimit)
			}

		default:
			// A symlink is the one member that could still point outside once
			// the skill is copied to its targets.
		}
	}
	return nil
}

func writeMember(root *os.Root, f *zip.File, name string, limit int64) (written int64, err error) {
	in, err := f.Open()
	if err != nil {
		return 0, err
	}
	defer in.Close()

	// Registry archives carry no Unix permissions, and Go reports 0666 for a
	// member that has none, so the mode is chosen here rather than believed.
	out, err := root.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, err
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
	}()

	return io.Copy(out, io.LimitReader(in, limit+1))
}
