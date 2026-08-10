// Package store owns the directories a scope keeps skills in. Each holds its own
// full copy, so no two of them ever have to agree about anything.
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Scope selects where skills are installed: the project (default) or the home
// directory (-g). Base is what the skill directories hang off of.
type Scope struct {
	Global bool
	Base   string
}

func NewScope(global bool) (Scope, error) {
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return Scope{}, fmt.Errorf("locate home directory: %w", err)
		}
		return Scope{Global: true, Base: home}, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return Scope{}, fmt.Errorf("locate working directory: %w", err)
	}
	return Scope{Base: cwd}, nil
}

func (s Scope) String() string {
	if s.Global {
		return "global"
	}
	return "local"
}

// AgentsDir is read natively by Amp, Cursor, Codex, Gemini CLI and others.
func (s Scope) AgentsDir() string {
	return filepath.Join(s.Base, ".agents", "skills")
}

// ClaudeDir is the only place Claude Code looks; it does not read .agents at all.
func (s Scope) ClaudeDir() string {
	if s.Global {
		if cfg := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); cfg != "" {
			return filepath.Join(cfg, "skills")
		}
	}
	return filepath.Join(s.Base, ".claude", "skills")
}

// Target is one directory that carries a copy of every skill. Label is short
// because it appears next to each skill in a list.
type Target struct {
	Label string
	Dir   string
}

// Targets is the set of directories kept in step, deliberately unrelated on
// disk. A user who has linked one to the other still works: both copies land in
// the same place, costing a second write and nothing else.
func (s Scope) Targets() []Target {
	return []Target{
		{Label: ".agents", Dir: s.AgentsDir()},
		{Label: ".claude", Dir: s.ClaudeDir()},
	}
}

func (t Target) Exists() bool {
	_, err := os.Lstat(t.Dir)
	return err == nil
}

// EnsureDirs creates every target directory, which is the whole of setup. With
// no invariant between them, there is no state it has to refuse.
func EnsureDirs(s Scope) ([]Target, error) {
	targets := s.Targets()
	for _, t := range targets {
		if err := os.MkdirAll(t.Dir, 0o755); err != nil {
			return nil, fmt.Errorf("create %s: %w", t.Dir, err)
		}
	}
	return targets, nil
}
