// Package cli wires the commands together and owns everything the user sees.
package cli

import (
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

// Execute runs the command tree. The caller reports the error.
func Execute() error {
	return newRootCmd().Execute()
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "skills",
		Short: "Install agent skills for every agent on this machine",
		Long: `skills installs agent skills into .agents/skills and .claude/skills.

Two conventions exist for where skills live: .agents/skills, read natively by
Amp, Cursor, Codex, Gemini CLI and others, and .claude/skills, the only place
Claude Code looks. Neither reads the other, so every skill is installed into
both and each directory stands on its own.

The default scope is the current project; -g uses your home directory.`,
		Version: version(),
		// A failed install is not a usage mistake, and dumping the full help
		// after one would bury the actual message. main prints errors itself.
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newAddCmd(), newListCmd(), newRemoveCmd())
	return root
}

func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "devel"
	}
	v := info.Main.Version
	if v == "" || v == "(devel)" {
		v = "devel"
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && len(s.Value) >= 7 {
			return v + "+" + s.Value[:7]
		}
	}
	return v
}

// truncate counts runes: a byte budget slices mid-rune. It is not display
// width, so a CJK column runs wide — that would need a width table.
func truncate(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if max < 1 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}
