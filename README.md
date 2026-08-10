# skills

Install agent skills into every directory the agents on your machine read.

```bash
skills add vercel-labs/agent-skills@web-design-guidelines
```

## Why two directories

A skill is a directory containing a `SKILL.md`. Two conventions exist for where
those directories live:

- `.agents/skills` — read natively by Amp, Cursor, Codex, Gemini CLI, Copilot,
  Cline and others.
- `.claude/skills` — the only place Claude Code looks. It does not read
  `.agents` at all.

Neither reads the other, so a skill installed in one is invisible to half your
tools. This installs into **both**, as two independent copies:

```
.agents/skills/web-design-guidelines/SKILL.md
.claude/skills/web-design-guidelines/SKILL.md
```

Nothing is shared, linked, or derived. The two directories never have to agree
about anything for the tool to work, which is the whole reason the design is
this boring: there is no invariant to establish, no state it can find and have
to refuse, and no symlink to break when a project is copied, archived, or
checked out on a machine that handles links differently.

The cost is drift. Edit one copy and the other is stale; `list` marks any skill
that is not in every directory so you can see it, and re-running `add` puts it
back everywhere.

## Usage

```
skills add [source] [-g] [-s NAME]... [--all]
skills list [-g]
skills remove <name>... [-g] [-y]
```

Default scope is the current project; `-g`/`--global` uses your home directory.
Every flag has a long form, and flags may appear before or after the source.
Run `skills <command> --help` for the full list.

### Sources

```bash
skills add owner/repo
skills add owner/repo@skill-name
skills add https://github.com/owner/repo
skills add https://github.com/owner/repo/tree/main/skills/web-design
skills add git@github.com:owner/repo.git      # or any git URL
skills add ./my-skills                        # or /abs/path, ~/path
```

`@skill-name` applies to repository sources only — a local path may legitimately
contain `@`, so use `-s NAME` there.

A `/tree/<ref>/` URL contributes its **subdirectory only**. The ref is reported
and ignored, and the default branch is cloned — a URL is an address to download
from, not a version to pin. This is what makes GitHub permalinks work: they name
a commit SHA, and honouring one meant `git clone --branch <sha>`, which cannot
succeed.

### Picking skills

A source with one skill installs it. A source with several lists them and
installs nothing, which is also how you browse what a repository offers:

```console
$ skills add vercel-labs/agent-skills
error: vercel-labs/agent-skills contains 9 skills; choose with -s NAME or --all

available:
  deploy-to-vercel  Deploy applications and websites to Vercel. Use when the us…
  vercel-optimize  Use for Vercel cost and performance optimization on deploye…
  web-design-guidelines  Review UI code for Web Interface Guidelines compliance. Use…
  ...
```

Then `-s NAME` (repeatable), `@NAME`, or `--all`.

Two skills that resolve to the same name install to the same directory, so the
last one wins — the same as re-running `add`.

### Naming

The frontmatter `name` decides the directory a skill installs as, sanitized down
to `[a-z0-9._-]`; without frontmatter the source directory name is used. From
then on **that directory name is the identity**. `list` prints it and `remove`
matches it exactly — a name that does not match is an error rather than
something to guess at, so `remove` can never delete a skill you did not name.

### Listing

`list` reads every directory and merges by name. A skill that is not in all of
them is marked, which is the one inconsistency this design can produce — a
skill an agent wrote into its own directory, or one added by hand:

```console
$ skills list
local
  /proj/.agents/skills
  /proj/.claude/skills
  note-taking                  Take structured notes.
  scratch-note                 Notes I keep meaning to file.  [.claude only]
  web-design-guidelines        Review UI code for Web Interface Guidelines com…

a marked skill is in some directories but not all;
re-run `skills add` with its source to put it in every one
```

`remove` deletes from every directory that holds a copy, and lists exactly those
paths before asking.

### Shell completion

`skills remove <TAB>` completes the names of installed skills, with their
descriptions:

```bash
skills completion fish > ~/.config/fish/completions/skills.fish
skills completion zsh  > "${fpath[1]}/_skills"
skills completion bash > /etc/bash_completion.d/skills
```

## Limits

- **No `update`.** Re-run `add` with the same source; it replaces the directory,
  clearing files deleted upstream. The new copy is staged and swapped in, so a
  failed run leaves the previous version intact rather than a half-written skill
  the agent would still read.
- **No lock file, and no ref pinning.** Nothing records where a skill came from
  and a source always resolves to the default branch, so keep a note of your
  sources if you want to refresh them later.
- **No converge command.** `list` shows a skill that is missing from some
  directory; putting it back means re-running `add` with its source. Copying
  between the directories would mean picking a winner when both hold the same
  name with different content, which is a decision this tool does not make.
- **`~/.claude/skills` does not reach cloud sessions.** Claude Code
  [does not sync it](https://code.claude.com/docs/en/skills) to Cowork, cloud
  sessions, or routines. Commit skills to a repository's `.claude/skills` for
  those — it is a real directory, so it commits and checks out unambiguously.
- **A newly created skills directory needs a restart.** Claude Code watches
  existing skill directories, but a top-level one created after startup is only
  picked up on restart.

## Install

```bash
go install github.com/aniaan/skills/cmd/skills@latest
```

Or from a checkout:

```bash
go build -o skills ./cmd/skills
```

## Development

```bash
go test ./...
go vet ./...
gofmt -l .
```

```
cmd/skills/       main, nothing but error reporting and the exit code
internal/skill/   what a skill is: discovery, frontmatter, name sanitizing
internal/source/  what `add` was given: argument parsing, git clone
internal/store/   the directories a scope installs into
internal/cli/     commands, flags, and everything the user sees
```

Dependencies run one way: `cli` → `source`, `store`; `store` → `skill`.

Three packages are linked into the binary:

| Dependency | Used for |
| --- | --- |
| [`goccy/go-yaml`](https://github.com/goccy/go-yaml) | frontmatter, in exactly one function (`skill.ParseFrontmatter`) |
| [`spf13/cobra`](https://github.com/spf13/cobra) | subcommands, help, shell completion |
| [`spf13/pflag`](https://github.com/spf13/pflag) | GNU-style flags — cobra's flag layer, not a separate choice |

`go list -m all` also shows `go-md2man`, `blackfriday` and `check.v1`. Those
belong to cobra's doc generator and tests and are never linked; `go list -deps`
confirms it.

Path containment is enforced with `os.Root` rather than string comparison, so a
hostile frontmatter name or a symlink planted in a skills directory cannot be
used to write outside it.
