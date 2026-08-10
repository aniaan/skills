package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/aniaan/skills/internal/skill"
)

// Installed is one skill present in a scope. Locations names the targets that
// hold a copy — missing from one is the only inconsistency this design allows.
type Installed struct {
	skill.Skill
	Scope     Scope
	Locations []string
}

func (i Installed) Everywhere(targets []Target) bool { return len(i.Locations) == len(targets) }

// List reads every target and merges by directory name. A skill in only one was
// put there by something else, such as an agent writing into .claude/skills.
func List(s Scope) ([]Installed, error) {
	targets := s.Targets()
	byName := make(map[string]*Installed)
	var order []string

	for _, t := range targets {
		found, err := listDir(t.Dir)
		if err != nil {
			return nil, err
		}
		for _, sk := range found {
			if existing, ok := byName[sk.Name]; ok {
				existing.Locations = append(existing.Locations, t.Label)
				if existing.Description == "" {
					existing.Description = sk.Description
				}
				continue
			}
			byName[sk.Name] = &Installed{
				Skill:     sk,
				Scope:     s,
				Locations: []string{t.Label},
			}
			order = append(order, sk.Name)
		}
	}

	out := make([]Installed, 0, len(order))
	for _, name := range order {
		out = append(out, *byName[name])
	}
	slices.SortFunc(out, func(a, b Installed) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

func listDir(dir string) ([]skill.Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}

	var out []skill.Skill
	for _, entry := range entries {
		// A sanitized name never begins with a dot, so a dotted entry is
		// plumbing — including staging left by an install that was killed.
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			continue
		}
		sk, err := skill.Load(path)
		if err != nil {
			continue
		}
		// The directory name is the identity: what the agent keys its command
		// off of, and what `remove` matches. Frontmatter only chose it.
		sk.Name = entry.Name()
		out = append(out, sk)
	}
	return out, nil
}

// Remove deletes skills from every target holding them. Absent everywhere is an
// error; absent from some is not. Names are matched exactly as List prints them,
// and containment comes from the *os.Root.
func Remove(s Scope, names []string) error {
	var errs []error

	for _, name := range names {
		removed := false
		for _, t := range s.Targets() {
			ok, err := removeFrom(t.Dir, name)
			if err != nil {
				errs = append(errs, fmt.Errorf("remove %s from %s: %w", name, t.Label, err))
			}
			removed = removed || ok
		}
		if !removed && len(errs) == 0 {
			errs = append(errs, fmt.Errorf("remove %s: not installed", name))
		}
	}
	return errors.Join(errs...)
}

// removeFrom reports whether it deleted anything, so the caller can tell a name
// that was nowhere from one already gone from this target.
func removeFrom(dir, name string) (bool, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	defer root.Close()

	// RemoveAll reports success for a name that is not there, so the Lstat is
	// what separates a real removal from a typo. An escaping name errors here.
	if _, err := root.Lstat(name); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if err := root.RemoveAll(name); err != nil {
		return false, err
	}
	return true, nil
}
