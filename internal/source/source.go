// Package source parses `skills add` arguments and materializes them on disk.
package source

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strings"
)

// fetcher fills an empty directory with a remote source. It is the only thing
// the kinds of remote source differ in — subpath, discovery and install are
// identical once the files are there — so a new kind is a constructor that
// returns one of these, not another branch in Fetch.
type fetcher func(ctx context.Context, dest string) error

// Source is a parsed `skills add` argument. Local is set for filesystem
// sources, CloneURL for everything else; Display is the original text.
type Source struct {
	Local    string
	CloneURL string
	// Parsed only to reach the subpath behind it, and to report it. Fetch
	// always takes the default branch, or a registry's latest version.
	IgnoredRef  string
	Subpath     string
	SkillFilter string
	Display     string

	fetch fetcher
}

func (s Source) IsLocal() bool { return s.Local != "" }

var (
	ownerRepo  = regexp.MustCompile(`^[\w.-]+/[\w.-]+$`)
	githubTree = regexp.MustCompile(`^https?://github\.com/([^/]+)/([^/]+)/tree/([^/]+)(?:/(.*))?$`)
	githubRepo = regexp.MustCompile(`^https?://github\.com/([^/]+)/([^/]+?)(?:\.git)?/?$`)
)

// Parse accepts owner/repo, owner/repo@skill, GitHub URLs with an optional
// /tree/<ref>/<subpath>, any git URL, and local paths. Only the subpath is used.
func Parse(arg string) (Source, error) {
	src, err := parseGit(arg)
	if err != nil || src.IsLocal() {
		return src, err
	}
	src.fetch = cloneInto(src.CloneURL)
	return src, nil
}

func parseGit(arg string) (Source, error) {
	display := arg
	if arg == "" {
		return Source{}, fmt.Errorf("empty source")
	}

	// Checked first so a path shaped like a shorthand is not read as a repo.
	// This is also why @skill does not apply to local paths: they may contain @.
	if isLocalPath(arg) {
		return Source{Local: arg, Display: display}, nil
	}

	arg, filter := splitSkillFilter(arg)
	src := Source{SkillFilter: filter, Display: display}

	if m := githubTree.FindStringSubmatch(arg); m != nil {
		src.CloneURL = fmt.Sprintf("https://github.com/%s/%s.git", m[1], strings.TrimSuffix(m[2], ".git"))
		src.IgnoredRef = m[3]
		src.Subpath = sanitizeSubpath(m[4])
		return src, nil
	}

	if m := githubRepo.FindStringSubmatch(arg); m != nil {
		src.CloneURL = fmt.Sprintf("https://github.com/%s/%s.git", m[1], m[2])
		return src, nil
	}

	if ownerRepo.MatchString(arg) {
		src.CloneURL = "https://github.com/" + strings.TrimSuffix(arg, ".git") + ".git"
		return src, nil
	}

	// GitLab, self-hosted, ssh:// and git@host:path pass through untouched.
	if isGitURL(arg) {
		src.CloneURL = arg
		return src, nil
	}

	return Source{}, fmt.Errorf("unrecognized source %q\n"+
		"expected owner/repo, a git URL, or a local path (./x, /abs/x)", display)
}

func isLocalPath(s string) bool {
	switch {
	case s == ".", s == "..":
		return true
	case strings.HasPrefix(s, "./"), strings.HasPrefix(s, "../"):
		return true
	case strings.HasPrefix(s, "/"), strings.HasPrefix(s, "~/"):
		return true
	}
	return false
}

func isGitURL(s string) bool {
	if strings.HasPrefix(s, "git@") || strings.Contains(s, "://") {
		return true
	}
	return strings.HasSuffix(s, ".git")
}

// splitSkillFilter peels a trailing `@skill-name` off a source. Only an `@`
// after the last `/` counts, so `git@github.com:owner/repo` stays intact.
func splitSkillFilter(s string) (rest, filter string) {
	slash := strings.LastIndex(s, "/")
	at := strings.LastIndex(s, "@")
	if at <= slash || at == len(s)-1 {
		return s, ""
	}
	return s[:at], s[at+1:]
}

// sanitizeSubpath normalizes a repo-relative path. An unsafe one yields "",
// meaning "search the whole repository".
func sanitizeSubpath(sub string) string {
	sub = strings.Trim(sub, "/")
	if sub == "" {
		return ""
	}
	clean := path.Clean(sub)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return ""
	}
	return clean
}
