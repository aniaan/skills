package cli

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/aniaan/skills/internal/store"
	"github.com/spf13/cobra"
)

func newRemoveCmd() *cobra.Command {
	var (
		global bool
		yes    bool
	)

	cmd := &cobra.Command{
		Use:               "remove <name>...",
		Aliases:           []string{"rm"},
		Short:             "Remove installed skills",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: completeInstalledSkills(&global),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemove(cmd, args, global, yes)
		},
	}

	cmd.Flags().BoolVarP(&global, "global", "g", false, "remove from the home directory instead of this project")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

func runRemove(cmd *cobra.Command, args []string, global, yes bool) error {
	out := cmd.OutOrStdout()

	scope, err := store.NewScope(global)
	if err != nil {
		return err
	}

	installed, err := store.List(scope)
	if err != nil {
		return err
	}

	doomed, err := resolveTargets(installed, args, scope)
	if err != nil {
		return err
	}

	// Only the directories that hold a copy: offering a path that is not there
	// asks the user to confirm a deletion that cannot happen.
	fmt.Fprintf(out, "removing from the %s scope:\n", scope)
	names := make([]string, 0, len(doomed))
	for _, sk := range doomed {
		names = append(names, sk.Name)
		for _, t := range scope.Targets() {
			if slices.Contains(sk.Locations, t.Label) {
				fmt.Fprintf(out, "  %s\n", filepath.Join(t.Dir, sk.Name))
			}
		}
	}

	if !yes && !confirm(cmd.InOrStdin(), out) {
		fmt.Fprintln(out, "aborted")
		return nil
	}

	if err := store.Remove(scope, names); err != nil {
		return err
	}
	fmt.Fprintf(out, "removed %d skill(s)\n", len(names))
	return nil
}

// resolveTargets validates every name first, so a typo in the second argument
// does not leave the first already gone. Matching is exact against `list`.
func resolveTargets(installed []store.Installed, args []string, scope store.Scope) ([]store.Installed, error) {
	present := make(map[string]store.Installed, len(installed))
	for _, sk := range installed {
		present[sk.Name] = sk
	}

	var targets []store.Installed
	for _, arg := range args {
		sk, ok := present[arg]
		if !ok {
			var b strings.Builder
			fmt.Fprintf(&b, "%q is not installed in the %s scope", arg, scope)
			if len(installed) == 0 {
				fmt.Fprintf(&b, "\n\nnothing is installed there")
			} else {
				b.WriteString("\n\ninstalled:")
				for _, sk := range installed {
					fmt.Fprintf(&b, "\n  %s", sk.Name)
				}
			}
			return nil, fmt.Errorf("%s", b.String())
		}
		if !slices.ContainsFunc(targets, func(t store.Installed) bool { return t.Name == arg }) {
			targets = append(targets, sk)
		}
	}
	return targets, nil
}

func confirm(in io.Reader, out io.Writer) bool {
	fmt.Fprint(out, "proceed? [y/N] ")
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}

// Errors are swallowed: a completion that cannot read the directory should
// offer nothing, not shout.
func completeInstalledSkills(global *bool) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, args []string, prefix string) ([]string, cobra.ShellCompDirective) {
		scope, err := store.NewScope(*global)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		installed, err := store.List(scope)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		var out []string
		for _, sk := range installed {
			if !strings.HasPrefix(sk.Name, prefix) {
				continue
			}
			if slices.Contains(args, sk.Name) {
				continue
			}
			out = append(out, sk.Name+"\t"+truncate(sk.Description, 50))
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	}
}
