package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScopeClaudeDirHonorsConfigEnv(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "/custom/config")

	global := Scope{Global: true, Base: "/home/user"}
	if got := global.ClaudeDir(); got != filepath.Join("/custom/config", "skills") {
		t.Errorf("global ClaudeDir = %q, want /custom/config/skills", got)
	}

	// The variable configures Claude Code's own directory, which is a global
	// notion; a project's .claude stays where the project is.
	local := Scope{Base: "/work/proj"}
	if got := local.ClaudeDir(); got != filepath.Join("/work/proj", ".claude", "skills") {
		t.Errorf("local ClaudeDir = %q, want /work/proj/.claude/skills", got)
	}
}

func TestEnsureDirs(t *testing.T) {
	scope := Scope{Base: t.TempDir()}

	// Idempotent: nothing here inspects the directories it is about to create,
	// so a second run has nothing to disagree with.
	for range 2 {
		targets, err := EnsureDirs(scope)
		if err != nil {
			t.Fatalf("EnsureDirs: %v", err)
		}
		if len(targets) != 2 {
			t.Fatalf("got %d targets, want 2", len(targets))
		}
		for _, target := range targets {
			info, err := os.Stat(target.Dir)
			if err != nil || !info.IsDir() {
				t.Errorf("%s (%s) missing: %v", target.Label, target.Dir, err)
			}
			if !target.Exists() {
				t.Errorf("%s reports missing after creation", target.Label)
			}
		}
	}
}

func TestEnsureDirsWithRelocatedClaudeDir(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "claude-config")
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)

	scope := Scope{Global: true, Base: t.TempDir()}
	if _, err := EnsureDirs(scope); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	if info, err := os.Stat(filepath.Join(cfg, "skills")); err != nil || !info.IsDir() {
		t.Errorf("relocated claude dir not created: %v", err)
	}
}

// A path that cannot be a directory is the only way setup fails now.
func TestEnsureDirsReportsAFileInTheWay(t *testing.T) {
	base := t.TempDir()
	scope := Scope{Base: base}
	if err := os.MkdirAll(filepath.Dir(scope.AgentsDir()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scope.AgentsDir(), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := EnsureDirs(scope); err == nil {
		t.Fatal("EnsureDirs succeeded, want an error")
	}
}
