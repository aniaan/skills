package cli

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/aniaan/skills/internal/skill"
	"github.com/aniaan/skills/internal/source"
	"github.com/aniaan/skills/internal/store"
	"github.com/spf13/cobra"
)

func newAddCmd() *cobra.Command {
	var (
		global bool
		names  []string
		all    bool
		hub    bool
	)

	cmd := &cobra.Command{
		Use:   "add [source]",
		Short: "Install skills from a repository or local path",
		Long: `Install skills into .agents/skills and .claude/skills.

Sources:
  owner/repo                                  GitHub shorthand
  owner/repo@skill-name                       one skill from that repository
  https://github.com/owner/repo
  https://github.com/owner/repo/tree/ref/path  a subdirectory; the ref is ignored
  git@github.com:owner/repo.git                any git URL
  ./path  ../path  /abs/path  ~/path           a local directory

With --hub the source is a URL copied from a skill registry's page rather than
a git repository, and the latest published version is installed.

Run with no source to create the skill directories without installing
anything.

When a repository holds several skills and none is named, they are all listed
and nothing is installed.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAdd(cmd, args, global, names, all, hub)
		},
	}

	cmd.Flags().BoolVarP(&global, "global", "g", false, "install into the home directory instead of this project")
	cmd.Flags().StringArrayVarP(&names, "skill", "s", nil, "skill to install; repeat for several")
	cmd.Flags().BoolVar(&all, "all", false, "install every skill found")
	cmd.Flags().BoolVar(&hub, "hub", false, "read the source as a skill registry URL, not a git repository")
	return cmd
}

func runAdd(cmd *cobra.Command, args []string, global bool, names []string, all, hub bool) error {
	out := cmd.OutOrStdout()

	scope, err := store.NewScope(global)
	if err != nil {
		return err
	}
	// Must be read before EnsureDirs creates them.
	newDirs := missingTargets(scope)

	targets, err := store.EnsureDirs(scope)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		fmt.Fprintf(out, "%s scope is ready:\n", scope)
		for _, t := range targets {
			fmt.Fprintf(out, "  %s\n", t.Dir)
		}
		hintRestart(cmd, newDirs)
		return nil
	}

	parse := source.Parse
	if hub {
		parse = source.ParseHub
	}
	src, err := parse(args[0])
	if err != nil {
		return err
	}
	if src.IgnoredRef != "" {
		fmt.Fprintf(out, "  note: ignoring %q; taking the latest\n", src.IgnoredRef)
	}

	dir, cleanup, err := source.Fetch(cmd.Context(), src)
	if err != nil {
		return err
	}
	defer cleanup()

	found, err := skill.Discover(dir)
	if err != nil {
		return fmt.Errorf("search %s: %w", src.Display, err)
	}

	if src.SkillFilter != "" {
		names = append(names, src.SkillFilter)
	}
	selected, err := selectSkills(found, names, all, src.Display)
	if err != nil {
		return err
	}

	for _, sk := range selected {
		if err := store.InstallAll(targets, sk); err != nil {
			return err
		}
		fmt.Fprintf(out, "  installed %s\n", sk.Name)
	}

	fmt.Fprintf(out, "\n%d skill(s) installed into the %s scope:\n", len(selected), scope)
	for _, t := range targets {
		fmt.Fprintf(out, "  %s\n", t.Dir)
	}
	hintRestart(cmd, newDirs)
	return nil
}

// selectSkills decides what to install. With no TUI, an ambiguous repository is
// an error listing every candidate — which doubles as the way to browse one.
func selectSkills(found []skill.Skill, names []string, all bool, display string) ([]skill.Skill, error) {
	if len(found) == 0 {
		return nil, fmt.Errorf("no %s found in %s", skill.File, display)
	}

	if all {
		return found, nil
	}

	if len(names) > 0 {
		var selected []skill.Skill
		for _, name := range names {
			sk, ok := findSkill(found, name)
			if !ok {
				return nil, fmt.Errorf("skill %q not found in %s\n\navailable:\n%s",
					name, display, availableList(found))
			}
			if !slices.ContainsFunc(selected, func(s skill.Skill) bool { return s.Dir == sk.Dir }) {
				selected = append(selected, sk)
			}
		}
		return selected, nil
	}

	if len(found) == 1 {
		return found, nil
	}

	return nil, fmt.Errorf("%s contains %d skills; choose with -s NAME or --all\n\navailable:\n%s",
		display, len(found), availableList(found))
}

// findSkill matches the installed name or the source directory name, so what a
// user reads in a repository tree works as well as what `list` shows.
func findSkill(found []skill.Skill, name string) (skill.Skill, bool) {
	want := skill.SanitizeName(name)
	for _, sk := range found {
		if sk.Name == want || skill.SanitizeName(filepath.Base(sk.Dir)) == want {
			return sk, true
		}
	}
	return skill.Skill{}, false
}

func availableList(found []skill.Skill) string {
	var b strings.Builder
	for _, sk := range found {
		fmt.Fprintf(&b, "  %s", sk.Name)
		if sk.Description != "" {
			fmt.Fprintf(&b, "  %s", truncate(sk.Description, 60))
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func missingTargets(scope store.Scope) []string {
	var missing []string
	for _, t := range scope.Targets() {
		if !t.Exists() {
			missing = append(missing, t.Dir)
		}
	}
	return missing
}

// Claude Code notices a new skill inside an existing directory, but not the
// directory itself appearing.
func hintRestart(cmd *cobra.Command, newDirs []string) {
	if len(newDirs) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "restart Claude Code to pick up the new skills directory")
	}
}
