package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aniaan/skills/internal/skill"
)

func TestInstallReplacesStaleFiles(t *testing.T) {
	canonical := t.TempDir()
	src := mkSkill(t, t.TempDir(), "foo", "---\nname: foo\n---\n")

	// A file from a previous install that upstream has since deleted. Merging
	// would leave the agent reading an instruction that no longer exists.
	stale := filepath.Join(canonical, "foo")
	mustMkdirAll(t, stale)
	if err := os.WriteFile(filepath.Join(stale, "STALE.md"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Install(canonical, skill.Skill{Name: "foo", Dir: src}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stale, "STALE.md")); !os.IsNotExist(err) {
		t.Errorf("stale file survived: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stale, skill.File)); err != nil {
		t.Errorf("SKILL.md missing: %v", err)
	}
}

func TestInstallCopiesNestedFilesAndModes(t *testing.T) {
	canonical := t.TempDir()
	src := mkSkill(t, t.TempDir(), "foo", "---\nname: foo\n---\n")

	mustMkdirAll(t, filepath.Join(src, "scripts"))
	script := filepath.Join(src, "scripts", "run.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Repository plumbing has no business inside an installed skill.
	mustMkdirAll(t, filepath.Join(src, ".git"))
	if err := os.WriteFile(filepath.Join(src, ".git", "config"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Install(canonical, skill.Skill{Name: "foo", Dir: src}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	info, err := os.Stat(filepath.Join(canonical, "foo", "scripts", "run.sh"))
	if err != nil {
		t.Fatalf("nested file missing: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %o, want 755", info.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(canonical, "foo", ".git")); !os.IsNotExist(err) {
		t.Errorf(".git was copied: %v", err)
	}
}

// A skill fetched from a repository may symlink to files that vanish with the
// temporary clone, so links are followed and their contents copied.
func TestInstallDereferencesSymlinks(t *testing.T) {
	canonical := t.TempDir()
	work := t.TempDir()
	src := mkSkill(t, work, "foo", "---\nname: foo\n---\n")

	shared := filepath.Join(work, "shared")
	mustMkdirAll(t, shared)
	if err := os.WriteFile(filepath.Join(shared, "note.md"), []byte("shared"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(shared, filepath.Join(src, "linked-dir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(shared, "note.md"), filepath.Join(src, "linked-file.md")); err != nil {
		t.Fatal(err)
	}
	// A broken link must not fail the whole install.
	if err := os.Symlink(filepath.Join(work, "gone"), filepath.Join(src, "dangling")); err != nil {
		t.Fatal(err)
	}

	if err := Install(canonical, skill.Skill{Name: "foo", Dir: src}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	dest := filepath.Join(canonical, "foo")
	for _, rel := range []string{"linked-dir/note.md", "linked-file.md"} {
		path := filepath.Join(dest, rel)
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatalf("%s missing: %v", rel, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			t.Errorf("%s was copied as a symlink, want a real file", rel)
		}
		body, err := os.ReadFile(path)
		if err != nil || string(body) != "shared" {
			t.Errorf("%s content = %q, %v", rel, body, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(dest, "dangling")); !os.IsNotExist(err) {
		t.Errorf("dangling link was copied: %v", err)
	}
}

func TestInstallSurvivesSymlinkLoop(t *testing.T) {
	canonical := t.TempDir()
	src := mkSkill(t, t.TempDir(), "foo", "---\nname: foo\n---\n")

	if err := os.Symlink(src, filepath.Join(src, "loop")); err != nil {
		t.Fatal(err)
	}

	if err := Install(canonical, skill.Skill{Name: "foo", Dir: src}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if _, err := os.Stat(filepath.Join(canonical, "foo", skill.File)); err != nil {
		t.Errorf("SKILL.md missing: %v", err)
	}
}

// `skills add ./.agents/skills` would otherwise delete the source during the
// clean step, before a single byte had been copied.
func TestInstallRefusesToOverwriteItsOwnSource(t *testing.T) {
	canonical := t.TempDir()
	src := mkSkill(t, canonical, "foo", "---\nname: foo\n---\n")

	if err := Install(canonical, skill.Skill{Name: "foo", Dir: src}); err == nil {
		t.Fatal("Install succeeded, want a refusal")
	}
	if _, err := os.Stat(filepath.Join(src, skill.File)); err != nil {
		t.Errorf("source was destroyed: %v", err)
	}
}

// The name comes from remote frontmatter, so it is attacker controlled.
func TestInstallCannotEscapeCanonicalDir(t *testing.T) {
	parent := t.TempDir()
	canonical := filepath.Join(parent, "skills")
	mustMkdirAll(t, canonical)

	outside := filepath.Join(parent, "outside")
	mustMkdirAll(t, outside)
	if err := os.WriteFile(filepath.Join(outside, "keep.md"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	src := mkSkill(t, t.TempDir(), "evil", "---\nname: ../../../etc/passwd\n---\n")
	sk, err := skill.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(canonical, sk); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Sanitizing flattened the traversal into a plain name inside the root.
	if _, err := os.Stat(filepath.Join(canonical, "etc-passwd", skill.File)); err != nil {
		t.Errorf("expected canonical/etc-passwd: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "keep.md")); err != nil {
		t.Errorf("escaped the canonical directory: %v", err)
	}
}

// os.Root refuses to follow a symlink out of the root, which a string prefix
// check cannot see.
func TestInstallRefusesSymlinkedDestination(t *testing.T) {
	parent := t.TempDir()
	canonical := filepath.Join(parent, "skills")
	mustMkdirAll(t, canonical)

	outside := filepath.Join(parent, "outside")
	mustMkdirAll(t, outside)
	if err := os.Symlink(outside, filepath.Join(canonical, "foo")); err != nil {
		t.Fatal(err)
	}

	src := mkSkill(t, t.TempDir(), "foo", "---\nname: foo\n---\n")
	if err := Install(canonical, skill.Skill{Name: "foo", Dir: src}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// The planted link was removed and replaced by a real directory; nothing
	// was written through it.
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Errorf("wrote through the symlink: %v entries, err %v", len(entries), err)
	}
	info, err := os.Lstat(filepath.Join(canonical, "foo"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("destination is still a symlink")
	}
}

// A skill is instructions an agent reads, so a half-written one is worse than
// an old one: the copy is staged and swapped in, never assembled in place.
func TestInstallLeavesThePreviousVersionOnFailure(t *testing.T) {
	canonical := t.TempDir()

	prev := filepath.Join(canonical, "foo")
	mustMkdirAll(t, prev)
	for _, name := range []string{skill.File, "reference.md"} {
		if err := os.WriteFile(filepath.Join(prev, name), []byte("v1"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	src := mkSkill(t, t.TempDir(), "foo", "---\nname: foo\n---\nv2\n")
	// Unreadable part-way through the copy.
	if err := os.WriteFile(filepath.Join(src, "unreadable.md"), []byte("x"), 0o000); err != nil {
		t.Fatal(err)
	}

	if err := Install(canonical, skill.Skill{Name: "foo", Dir: src}); err == nil {
		t.Fatal("Install succeeded, want a copy failure")
	}

	for _, name := range []string{skill.File, "reference.md"} {
		body, err := os.ReadFile(filepath.Join(prev, name))
		if err != nil {
			t.Errorf("%s lost: %v", name, err)
			continue
		}
		if string(body) != "v1" {
			t.Errorf("%s = %q, want the previous version", name, body)
		}
	}
}

// Staging must not accumulate, and must not be mistaken for a skill if a run is
// killed before the swap.
func TestInstallCleansUpStaging(t *testing.T) {
	base := t.TempDir()
	scope := Scope{Base: base}
	canonical := scope.AgentsDir()
	mustMkdirAll(t, canonical)

	src := mkSkill(t, t.TempDir(), "foo", "---\nname: foo\ndescription: d\n---\n")
	if err := Install(canonical, skill.Skill{Name: "foo", Dir: src}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	entries, err := os.ReadDir(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "foo" {
		t.Errorf("canonical holds %v, want just [foo]", entries)
	}

	// A staging directory left by a killed run carries a partial SKILL.md, so
	// List must not report it as installed.
	leftover := filepath.Join(canonical, stagingName("bar"))
	mustMkdirAll(t, leftover)
	if err := os.WriteFile(filepath.Join(leftover, skill.File), []byte("---\nname: bar\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	installed, err := List(scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(installed) != 1 || installed[0].Name != "foo" {
		t.Errorf("List reported %+v, want just foo", installed)
	}
}

// Every target holds a copy, so a skill installed by this tool is listed once
// with no marker; Locations is what tells the two cases apart.
func TestListMergesTargets(t *testing.T) {
	scope := Scope{Base: t.TempDir()}
	targets, err := EnsureDirs(scope)
	if err != nil {
		t.Fatal(err)
	}

	src := mkSkill(t, t.TempDir(), "everywhere", "---\nname: everywhere\ndescription: d\n---\n")
	if err := InstallAll(targets, skill.Skill{Name: "everywhere", Dir: src}); err != nil {
		t.Fatal(err)
	}
	// Only in .claude, as if Claude Code had written it there itself.
	mkSkill(t, scope.ClaudeDir(), "claude-only", "---\nname: claude-only\ndescription: c\n---\n")
	// Only in .agents, as if it had been dropped in by hand.
	mkSkill(t, scope.AgentsDir(), "agents-only", "---\nname: agents-only\ndescription: a\n---\n")

	installed, err := List(scope)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(installed) != 3 {
		t.Fatalf("got %d skills, want 3: %+v", len(installed), installed)
	}

	want := map[string][]string{
		"agents-only": {".agents"},
		"claude-only": {".claude"},
		"everywhere":  {".agents", ".claude"},
	}
	for _, sk := range installed {
		if got := strings.Join(sk.Locations, ","); got != strings.Join(want[sk.Name], ",") {
			t.Errorf("%s locations = %v, want %v", sk.Name, sk.Locations, want[sk.Name])
		}
		if sk.Everywhere(targets) != (len(want[sk.Name]) == len(targets)) {
			t.Errorf("%s Everywhere = %v", sk.Name, sk.Everywhere(targets))
		}
	}
}

// A user who linked one directory to the other still works: both copies land in
// the same place, and the skill is reported once, present in both.
func TestListWithOneTargetLinkedToTheOther(t *testing.T) {
	base := t.TempDir()
	scope := Scope{Base: base}
	mustMkdirAll(t, scope.AgentsDir())
	mustMkdirAll(t, filepath.Dir(scope.ClaudeDir()))
	if err := os.Symlink(filepath.Join("..", ".agents", "skills"), scope.ClaudeDir()); err != nil {
		t.Fatal(err)
	}

	targets, err := EnsureDirs(scope)
	if err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	src := mkSkill(t, t.TempDir(), "foo", "---\nname: foo\n---\n")
	if err := InstallAll(targets, skill.Skill{Name: "foo", Dir: src}); err != nil {
		t.Fatalf("InstallAll: %v", err)
	}

	installed, err := List(scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(installed) != 1 {
		t.Fatalf("got %d skills, want 1: %+v", len(installed), installed)
	}
	if !installed[0].Everywhere(targets) {
		t.Errorf("locations = %v, want both", installed[0].Locations)
	}
}

func TestListUsesDirectoryNames(t *testing.T) {
	base := t.TempDir()
	scope := Scope{Base: base}
	canonical := scope.AgentsDir()
	mustMkdirAll(t, canonical)

	// The directory name is what the agent keys its command off of, so that is
	// what list reports even when frontmatter disagrees.
	mkSkill(t, canonical, "on-disk-name", "---\nname: Different Name\ndescription: d\n---\n")
	mkSkill(t, canonical, "alpha", "---\nname: alpha\ndescription: first\n---\n")
	if err := os.WriteFile(filepath.Join(canonical, "loose.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustMkdirAll(t, filepath.Join(canonical, "not-a-skill"))

	installed, err := List(scope)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(installed) != 2 {
		t.Fatalf("got %d skills, want 2: %+v", len(installed), installed)
	}
	if installed[0].Name != "alpha" || installed[1].Name != "on-disk-name" {
		t.Errorf("names = %q, %q", installed[0].Name, installed[1].Name)
	}
	if installed[0].Description != "first" {
		t.Errorf("description = %q, want %q", installed[0].Description, "first")
	}
}

func TestListMissingDirIsEmpty(t *testing.T) {
	installed, err := List(Scope{Base: t.TempDir()})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(installed) != 0 {
		t.Errorf("got %d skills, want 0", len(installed))
	}
}

// Removal has to reach every copy, or the skill comes back the next time an
// agent that reads the other directory runs.
func TestRemoveClearsEveryTarget(t *testing.T) {
	scope := Scope{Base: t.TempDir()}
	targets, err := EnsureDirs(scope)
	if err != nil {
		t.Fatal(err)
	}

	src := mkSkill(t, t.TempDir(), "foo", "---\nname: foo\n---\n")
	keep := mkSkill(t, t.TempDir(), "bar", "---\nname: bar\n---\n")
	for _, sk := range []skill.Skill{{Name: "foo", Dir: src}, {Name: "bar", Dir: keep}} {
		if err := InstallAll(targets, sk); err != nil {
			t.Fatal(err)
		}
	}

	if err := Remove(scope, []string{"foo"}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	for _, target := range targets {
		if _, err := os.Stat(filepath.Join(target.Dir, "foo")); !os.IsNotExist(err) {
			t.Errorf("foo survived in %s: %v", target.Label, err)
		}
		if _, err := os.Stat(filepath.Join(target.Dir, "bar")); err != nil {
			t.Errorf("bar was removed from %s too: %v", target.Label, err)
		}
	}
}

// A skill only one target holds — written there by an agent, or made by hand —
// still comes out, and its absence elsewhere is not an error.
func TestRemoveAcceptsASkillInOneTarget(t *testing.T) {
	scope := Scope{Base: t.TempDir()}
	if _, err := EnsureDirs(scope); err != nil {
		t.Fatal(err)
	}
	mkSkill(t, scope.ClaudeDir(), "written-in-place", "---\nname: written-in-place\n---\n")

	if err := Remove(scope, []string{"written-in-place"}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(scope.ClaudeDir(), "written-in-place")); !os.IsNotExist(err) {
		t.Errorf("survived: %v", err)
	}
}

// RemoveAll reports success for a name that is not there, which would let a
// typo be counted and printed as a removal.
func TestRemoveReportsANameThatIsNowhere(t *testing.T) {
	scope := Scope{Base: t.TempDir()}
	if _, err := EnsureDirs(scope); err != nil {
		t.Fatal(err)
	}
	mkSkill(t, scope.AgentsDir(), "foo", "---\nname: foo\n---\n")

	if err := Remove(scope, []string{"nope"}); err == nil {
		t.Fatal("Remove succeeded, want an error")
	}
}

// The name reaches Remove unsanitized, so containment rests entirely on the
// *os.Root.
func TestRemoveCannotEscape(t *testing.T) {
	base := t.TempDir()
	scope := Scope{Base: base}
	if _, err := EnsureDirs(scope); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(base, "outside")
	mkSkill(t, outside, "victim", "---\nname: victim\n---\n")

	for _, name := range []string{"../outside", "../outside/victim", "..", "/etc"} {
		if err := Remove(scope, []string{name}); err == nil {
			t.Errorf("Remove(%q) succeeded, want a refusal", name)
		}
	}
	if _, err := os.Stat(filepath.Join(outside, "victim", skill.File)); err != nil {
		t.Errorf("escaped the target directories: %v", err)
	}
}

func mkSkill(t *testing.T, parent, name, content string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, skill.File), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}
