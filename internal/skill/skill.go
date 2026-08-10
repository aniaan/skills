// Package skill discovers and parses skill directories: a directory holding a
// SKILL.md with YAML frontmatter.
package skill

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
)

// File is the marker that makes a directory a skill.
const File = "SKILL.md"

// SkipDirs are never descended into when searching, nor copied into an install.
var SkipDirs = []string{".git", "node_modules", "dist", "build", "__pycache__"}

// Skill is a directory containing a SKILL.md. Name is the sanitized directory
// it installs as; Description is display-only.
type Skill struct {
	Name        string
	Description string
	Dir         string
}

// ParseFrontmatter is the only place YAML is parsed. Missing frontmatter is not
// an error; callers fall back to the directory name.
func ParseFrontmatter(raw []byte) (name, description string, err error) {
	block, ok := frontmatterBlock(raw)
	if !ok {
		return "", "", nil
	}

	// Other fields (allowed-tools, user-invocable, ...) are deliberately ignored.
	var fm struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal(block, &fm); err != nil {
		return "", "", fmt.Errorf("parse frontmatter: %w", err)
	}
	return strings.TrimSpace(fm.Name), strings.TrimSpace(fm.Description), nil
}

// frontmatterBlock returns the bytes between the two `---` fences. Only a fence
// on the very first line counts, matching the upstream CLI.
func frontmatterBlock(raw []byte) ([]byte, bool) {
	s := strings.TrimPrefix(string(raw), "\ufeff")

	rest, ok := strings.CutPrefix(s, "---\n")
	if !ok {
		if rest, ok = strings.CutPrefix(s, "---\r\n"); !ok {
			return nil, false
		}
	}

	for i := 0; i < len(rest); {
		end := strings.IndexByte(rest[i:], '\n')
		line := rest[i:]
		if end >= 0 {
			line = rest[i : i+end]
		}
		if strings.TrimRight(line, "\r") == "---" {
			return []byte(rest[:i]), true
		}
		if end < 0 {
			break
		}
		i += end + 1
	}
	return nil, false
}

var (
	// Collapsing disallowed runs to one hyphen turns `../` and spaces into
	// harmless separators.
	nonNameChars = regexp.MustCompile(`[^a-z0-9._]+`)
	// Trailing dots and hyphens would give hidden or oddly-named directories.
	trimNameEdges = regexp.MustCompile(`^[.\-]+|[.\-]+$`)
)

// SanitizeName runs once, at install time, on attacker-controlled frontmatter.
// It only tidies; containment comes from writing through an *os.Root.
func SanitizeName(name string) string {
	s := strings.ToLower(name)
	s = nonNameChars.ReplaceAllString(s, "-")
	s = trimNameEdges.ReplaceAllString(s, "")
	if len(s) > 255 {
		s = s[:255]
	}
	if s == "" {
		return "unnamed-skill"
	}
	return s
}

// Load prefers the frontmatter `name` and falls back to the directory name,
// matching the upstream CLI.
func Load(dir string) (Skill, error) {
	raw, err := os.ReadFile(filepath.Join(dir, File))
	if err != nil {
		return Skill{}, err
	}
	name, description, err := ParseFrontmatter(raw)
	if err != nil {
		return Skill{}, fmt.Errorf("%s: %w", filepath.Join(dir, File), err)
	}
	if name == "" {
		name = filepath.Base(dir)
	}
	return Skill{
		Name:        SanitizeName(name),
		Description: description,
		Dir:         dir,
	}, nil
}

// Discover returns every skill directory beneath root, sorted by name. A
// SKILL.md nested inside a skill is a supporting file, not a second skill.
func Discover(root string) ([]Skill, error) {
	var skills []Skill

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && slices.Contains(SkipDirs, d.Name()) {
			return fs.SkipDir
		}

		info, statErr := os.Stat(filepath.Join(path, File))
		if statErr != nil || !info.Mode().IsRegular() {
			return nil
		}

		sk, loadErr := Load(path)
		if loadErr != nil {
			fmt.Fprintf(os.Stderr, "warning: skipped %s: %v\n", path, loadErr)
			return fs.SkipDir
		}
		skills = append(skills, sk)
		return fs.SkipDir
	})
	if err != nil {
		return nil, err
	}

	slices.SortFunc(skills, func(a, b Skill) int { return strings.Compare(a.Name, b.Name) })
	return skills, nil
}
