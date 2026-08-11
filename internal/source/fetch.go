package source

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Generous rather than tight, for large repositories on slow links.
const cloneTimeout = 5 * time.Minute

// Fetch puts a Source on local disk and returns the directory to search plus a
// cleanup func for any temporary clone.
func Fetch(ctx context.Context, src Source) (dir string, cleanup func(), err error) {
	if src.IsLocal() {
		abs, err := resolveLocal(src.Local)
		if err != nil {
			return "", nil, err
		}
		return abs, func() {}, nil
	}

	// Only Parse and ParseHub can supply one, so this is a source that never
	// came from either — an error rather than a nil call.
	if src.fetch == nil {
		return "", nil, fmt.Errorf("no way to fetch %s", src.Display)
	}

	tmp, err := os.MkdirTemp("", "skills-")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}
	cleanup = func() { os.RemoveAll(tmp) }

	if err := src.fetch(ctx, tmp); err != nil {
		cleanup()
		return "", nil, err
	}

	root := tmp
	if src.Subpath != "" {
		root = filepath.Join(tmp, filepath.FromSlash(src.Subpath))
		if info, err := os.Stat(root); err != nil || !info.IsDir() {
			cleanup()
			return "", nil, fmt.Errorf("subpath %q not found in %s", src.Subpath, src.Display)
		}
	}
	return root, cleanup, nil
}

// cloneInto always takes the default branch. --branch accepts branch and tag
// names only, so honouring a ref broke every GitHub permalink, which pins a SHA.
func cloneInto(url string) fetcher {
	return func(ctx context.Context, dest string) error {
		ctx, cancel := context.WithTimeout(ctx, cloneTimeout)
		defer cancel()

		// -- keeps a source that begins with a dash in the URL position instead
		// of git's option position, where --upload-pack would run a command.
		args := []string{"clone", "--depth", "1", "--", url, dest}

		cmd := exec.CommandContext(ctx, "git", args...)
		// A blocked credential prompt looks identical to a network stall, so
		// never let git ask.
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		var stderr strings.Builder
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("clone %s timed out after %s", url, cloneTimeout)
			}
			detail := strings.TrimSpace(stderr.String())
			if detail == "" {
				detail = err.Error()
			}
			return fmt.Errorf("clone %s failed: %s", url, detail)
		}
		return nil
	}
}

func resolveLocal(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve %s: %w", p, err)
		}
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", p, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", p, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", abs)
	}
	return abs, nil
}
