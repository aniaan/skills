# skills

Install agent skills into both `.agents/skills` and `.claude/skills`, as two
independent copies. Neither directory reads the other, and Claude Code only
looks at `.claude`.

```bash
skills add vercel-labs/agent-skills@web-design-guidelines
```

## Commands

```
skills add [source] [-g] [-s NAME]... [--all]
skills list [-g]
skills remove <name>... [-g] [-y]
```

`-g` uses your home directory instead of the current project. `list` marks any
skill that is not in every directory; re-run `add` to put it back.

## Sources

A source names its transport as a prefix. `git+` is the default and may be left
off; `hub+` reads a skill registry — ClawHub or anything serving its API —
because the URL alone cannot tell you what is on the other end.

```bash
skills add owner/repo
skills add owner/repo@skill-name              # one skill; use -s NAME for local paths
skills add https://github.com/owner/repo/tree/main/skills/web-design
skills add git@github.com:owner/repo.git      # or any git URL
skills add ./my-skills                        # or /abs/path, ~/path
skills add hub+https://hub.example.com/owner/skill-name
```

A source holding several skills lists them and installs nothing; pick with
`-s NAME`, `@NAME`, or `--all`. Refs and versions are always ignored — a source
resolves to the default branch, or to a registry's latest.

The frontmatter `name` becomes the installed directory name, and that name is
what `list` prints and `remove` matches.

## Gotchas

- **No `update`, no lock file, no converge.** Re-run `add` with the same source,
  and keep your own note of what that source was.
- **Registry archives carry no file modes** — everything extracts `0644`, so a
  script arrives without its executable bit.
- **`~/.claude/skills` does not reach cloud sessions.** Commit skills to a
  repository's `.claude/skills` for those.
- **A new skills directory needs a restart** of Claude Code.

## Install

```bash
go install github.com/aniaan/skills/cmd/skills@latest
```

## Development

```bash
go test ./... && go vet ./... && gofmt -l .
```

```
cmd/skills/       main, error reporting and the exit code
internal/skill/   what a skill is: discovery, frontmatter, name sanitizing
internal/source/  what `add` was given: parsing, git clone, registry download
internal/store/   the directories a scope installs into
internal/cli/     commands, flags, and everything the user sees
```

`cli` → `source`, `store`; `store` → `skill`. Writes go through an `os.Root`, so
neither a hostile frontmatter name nor a zip member escapes its directory.
