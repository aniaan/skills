package store

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/aniaan/skills/internal/skill"
)

// InstallAll puts one skill in every target. The copies are independent, so a
// failing target is reported rather than allowed to block the others.
func InstallAll(targets []Target, sk skill.Skill) error {
	var errs []error
	for _, t := range targets {
		if err := Install(t.Dir, sk); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", t.Label, err))
		}
	}
	return errors.Join(errs...)
}

// Install copies one skill into a directory, replacing any previous version.
// It stages and swaps rather than filling in place: a half-written skill is
// worse than a stale one. Writing through an *os.Root enforces containment,
// including against symlinks a prefix check cannot see.
func Install(dir string, sk skill.Skill) error {
	dest := filepath.Join(dir, sk.Name)
	if overlaps(sk.Dir, dest) {
		return fmt.Errorf("refusing to install %s onto itself (%s)", sk.Dir, dest)
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("open %s: %w", dir, err)
	}
	defer root.Close()

	staging := stagingName(sk.Name)
	if err := root.RemoveAll(staging); err != nil {
		return fmt.Errorf("clean %s: %w", filepath.Join(dir, staging), err)
	}
	if err := root.MkdirAll(staging, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Join(dir, staging), err)
	}
	defer root.RemoveAll(staging)

	if err := copyInto(root, sk.Dir, staging, map[string]bool{}); err != nil {
		return fmt.Errorf("copy %s: %w", sk.Name, err)
	}

	// Replace, not merge, so a file deleted upstream cannot linger. Rename will
	// not overwrite a populated directory, so the old one goes first.
	if err := root.RemoveAll(sk.Name); err != nil {
		return fmt.Errorf("clean %s: %w", dest, err)
	}
	if err := root.Rename(staging, sk.Name); err != nil {
		return fmt.Errorf("install %s: %w", dest, err)
	}
	return nil
}

// stagingName derives the sibling to build the new copy in. A sanitized name
// never starts with a dot, so the prefix cannot collide with a real skill.
func stagingName(name string) string {
	const prefix = ".staging-"
	if len(name) > 255-len(prefix) { // NAME_MAX
		name = name[:255-len(prefix)]
	}
	return prefix + name
}

// copyInto recursively copies srcDir into destRel inside root. Stat, not Lstat:
// links into a temp clone would dangle once it is removed. chain holds the
// resolved directories on the current path so a link loop terminates.
func copyInto(root *os.Root, srcDir, destRel string, chain map[string]bool) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		name := entry.Name()
		srcPath := filepath.Join(srcDir, name)
		destPath := filepath.Join(destRel, name)

		info, err := os.Stat(srcPath)
		if err != nil {
			// A broken symlink in the source is not worth failing the install.
			continue
		}

		switch {
		case info.IsDir():
			if slices.Contains(skill.SkipDirs, name) {
				continue
			}
			real := resolved(srcPath)
			if chain[real] {
				continue
			}
			chain[real] = true
			if err := root.MkdirAll(destPath, 0o755); err != nil {
				return err
			}
			if err := copyInto(root, srcPath, destPath, chain); err != nil {
				return err
			}
			delete(chain, real)

		case info.Mode().IsRegular():
			if err := copyFile(root, srcPath, destPath, info.Mode().Perm()); err != nil {
				return err
			}

		default:
			// Sockets, devices and fifos have no meaning inside a skill.
		}
	}
	return nil
}

func copyFile(root *os.Root, src, destRel string, perm fs.FileMode) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := root.OpenFile(destRel, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
	}()

	_, err = io.Copy(out, in)
	return err
}

// overlaps reports whether either path contains the other, with symlinks
// resolved so two names for one directory are caught.
func overlaps(a, b string) bool {
	ra, rb := resolved(a), resolved(b)
	if ra == rb {
		return true
	}
	return strings.HasPrefix(ra, rb+string(filepath.Separator)) ||
		strings.HasPrefix(rb, ra+string(filepath.Separator))
}

// resolved evaluates symlinks, falling back to the parent so a destination that
// does not exist yet still compares correctly.
func resolved(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	parent, base := filepath.Split(path)
	if real, err := filepath.EvalSymlinks(parent); err == nil {
		return filepath.Join(real, base)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}
