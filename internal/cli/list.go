package cli

import (
	"fmt"
	"strings"

	"github.com/aniaan/skills/internal/store"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var global bool

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List installed skills",
		Long: `List installed skills.

Without -g both scopes are shown, because an agent working inside a project
loads both and one column is half the picture.

Every skill is kept in every directory of its scope. One that is present in
only some of them is marked, since that is the one inconsistency this can have:
a skill an agent wrote into its own directory, or one added by hand.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, global)
		},
	}

	cmd.Flags().BoolVarP(&global, "global", "g", false, "list only the home directory scope")
	return cmd
}

func runList(cmd *cobra.Command, global bool) error {
	out := cmd.OutOrStdout()

	scopes := make([]store.Scope, 0, 2)
	if !global {
		local, err := store.NewScope(false)
		if err != nil {
			return err
		}
		scopes = append(scopes, local)
	}
	globalScope, err := store.NewScope(true)
	if err != nil {
		return err
	}
	scopes = append(scopes, globalScope)

	total := 0
	partial := false
	for _, scope := range scopes {
		installed, err := store.List(scope)
		if err != nil {
			return err
		}
		total += len(installed)

		targets := scope.Targets()
		fmt.Fprintf(out, "\n%s\n", scope)
		for _, t := range targets {
			fmt.Fprintf(out, "  %s\n", t.Dir)
		}
		if len(installed) == 0 {
			fmt.Fprintln(out, "  (none)")
			continue
		}
		for _, sk := range installed {
			mark := ""
			if !sk.Everywhere(targets) {
				partial = true
				mark = fmt.Sprintf("  [%s only]", strings.Join(sk.Locations, ", "))
			}
			fmt.Fprintf(out, "  %-28s %s%s\n", sk.Name, truncate(sk.Description, 60), mark)
		}
	}

	if total == 0 {
		fmt.Fprintln(out, "\nnothing installed yet — try: skills add owner/repo")
	}
	if partial {
		fmt.Fprintln(out, "\na marked skill is in some directories but not all;")
		fmt.Fprintln(out, "re-run `skills add` with its source to put it in every one")
	}
	return nil
}
