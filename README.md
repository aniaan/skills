# skills

Install agent skills into `.agents/skills` and `.claude/skills` as two
independent copies.

## Usage

```
skills add [source] [-g] [-s NAME]... [--all]
skills list [-g]
skills remove <name>... [-g] [-y]
```

| Flag | |
| --- | --- |
| `-g`, `--global` | `add` and `remove` use your home directory instead of the current project; `list` shows only the home scope, which it otherwise lists alongside the project |
| `-s`, `--skill NAME` | install one named skill; repeat for several |
| `--all` | install every skill the source holds |
| `-y`, `--yes` | skip the removal prompt |

In the global scope, `$CLAUDE_CONFIG_DIR/skills` replaces `~/.claude/skills`
when that variable is set.

`list` and `remove` have the aliases `ls` and `rm`. `skills completion <shell>`
writes a completion script.

## Sources

```bash
skills add owner/repo
skills add owner/repo@skill-name
skills add https://github.com/owner/repo
skills add https://github.com/owner/repo/tree/main/skills/web-design
skills add git@github.com:owner/repo.git
skills add ./my-skills
skills add hub+https://hub.example.com/owner/skill-name
```

`git+` is the default transport prefix and may be left off. `hub+` reads a skill
registry serving the ClawHub API.

`@skill-name` selects one skill from a repository source; use `-s NAME` for
local paths.

A `/tree/<ref>/<subpath>` URL installs that subdirectory only; without a subpath
the whole repository is searched. Refs and versions are reported and then
ignored, whether written as a `/tree/<ref>/`, a `@version` suffix, or a
`?version=` query: a git source resolves to the default branch, a registry
source to its latest published version.

A source holding several skills lists them and installs nothing until one is
chosen with `-s NAME`, `@NAME`, or `--all`.

The installed directory name comes from the frontmatter `name`, sanitized to
`[a-z0-9._-]`, or from the source directory name when there is no frontmatter.
`list` prints that name and `remove` matches it exactly.

## Notes

- There is no `update`, lock file, or version pinning. Re-run `add`.
- `list` marks a skill that is not present in every directory.
- Registry archives carry no file modes; files are created `0644` before the
  umask applies, so scripts arrive without their executable bit.
- `~/.claude/skills` is not synced to Claude Code cloud sessions.
- A newly created skills directory is picked up after restarting Claude Code.

## Install

```bash
go install github.com/aniaan/skills/cmd/skills@latest
```

## Development

```bash
go test ./... && go vet ./... && gofmt -l .
```

```
cmd/skills/       main
internal/skill/   discovery, frontmatter, name sanitizing
internal/source/  argument parsing, git clone, registry download
internal/store/   the directories a scope installs into
internal/cli/     commands and flags
```
