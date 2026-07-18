# North

[![CI](https://github.com/SamP-S/north/actions/workflows/ci.yml/badge.svg)](https://github.com/SamP-S/north/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE.md)

North is an **in-repo Markdown task board** with a CLI, modeled on
[Backlog.md](https://github.com/MrLesk/Backlog.md). The board lives in a `north/`
directory committed inside your own project repo — each task is a plain Markdown
file. There is no daemon and no central state: `north <cmd>` operates directly on
the files, and **git is entirely yours** (North never pushes or pulls).

---

## Install
Requires [Go](https://go.dev/dl/) 1.25+:

```bash
go install github.com/SamP-S/north/cmd/north@latest
# or, from a clone: make install
```

One self-contained binary — no runtime, no daemon. If `north` is "command not
found" afterwards, add Go's bin dir to your PATH:
`export PATH="$PATH:$(go env GOPATH)/bin"`.

## Quick start
```bash
cd your-project
north init                                  # create the north/ board
north task create "Add login form" --assignee claude:opus --labels auth   # lands in drafts/
north task state 1 active                   # drafts -> tasks (onto the active board)
north task move 1 in_progress               # change status (any → any)
north task view 1
north task move 1 done
north board                                 # active counts per status
```

---

## Two axes: state and status
A task has two independent properties. Each takes one value from a fixed
set, but movement between values is free — any value to any other value in a
single command:

- **State** — its lifecycle *location* (the folder): `draft`, `active`,
  `archive`. Changed with `north task state <id> <state>`.
- **Status** — its workflow *column* (frontmatter, shown on the board while
  active):
  `ready`, `in_progress`, `blocked`, `done`, `failed`. Changed with
  `north task move <id> <status>`.

New tasks start as a **draft** (status `ready`). State moves relocate the
file and preserve status; `move` rewrites status in place — in any state,
though status only shows on the board once the task is active.

## The board
`north init` scaffolds, inside your repo:
```
north/
  config.yml         # board marker + settings (version, auto_commit)
  task-template.md   # body scaffold for bodyless creates (yours to edit)
  .gitattributes     # "* text eol=lf" — keeps board files LF on every clone
  .gitignore         # keeps the transient .lock and *.tmp files out of git
  drafts/            # state: draft
  tasks/             # state: active   (status in frontmatter)
  archive/           # state: archive
```
A task is one file, `<n>-<title-slug>.md`, in its state folder. `config.yml`
carries a `version: 1` format stamp; a board written by a newer North is
refused rather than misread. Creating a task without `--body` fills it from
`task-template.md` — edit or delete the file to change what new tasks start
with; North never parses it back. Mutating commands take a brief advisory
lock (`north/.lock`), and task ids are never reused.

### Task file
```yaml
---
id: "12"                 # bare number, unique across the board
title: Add login form
status: ready            # workflow status (frontmatter is the source of truth)
assignee: claude:opus    # optional, free-form — a person ("john") or an agent
labels: [auth]           # optional free-form tags
depends_on: ["4"]        # task ids
created_at: "2026-06-24T…"
updated_at: "2026-06-24T…"
---
Free-form body: description, plan, notes, blockers, results — your structure.
```
Custom frontmatter keys you add by hand are preserved by every North rewrite.
Malformed files never break the board — they surface as warnings, and
`north doctor` can diagnose and repair board-level problems.

---

## CLI
| Command | Description |
|---|---|
| `north init` | Scaffold the board (refuses if one already exists at or above cwd) |
| `north task create <title> [--assignee --labels --depends-on --body \| --body-file]` | Create a task (drafts/) |
| `north task list [--state …] [--status S] [--assignee A] [--deps met\|unmet] [--search TEXT] [--label L] [--sort …] [--reverse] [-l N]` | List tasks (default active, newest first) |
| `north task view <id>` | Show a task |
| `north task edit <id> [--title --assignee --labels --depends-on --body \| --body-file \| --append-body]` | Edit a task |
| `north task move <id[,id…]> <status>` | Set status (any → any, in any state) |
| `north task state <id[,id…]> <draft\|active\|archive>` | Set lifecycle state (any → any) |
| `north task delete <id[,id…]> [-y]` | Delete tasks (`-y` required in machine/non-TTY modes and for batches) |
| `north next [-l N] [--label L]` | Show the next workable task(s) (active, ready, unassigned, deps met; `-l 0` = all) — read-only |
| `north take [id] [--assignee A] [--label L]` | Atomically claim the next workable task (or a specific id) — `in_progress` + assignee in one lock hold |
| `north board` | Active counts per status + draft/archive tally |
| `north cleanup [--older-than DAYS] [--dry-run]` | Archive active done tasks (`--dry-run` previews) |
| `north doctor [--fix]` | Board integrity check (report only — exits 0; `--fix` repairs what is safe) |
| `north config list\|get\|set` | Board settings (`auto_commit`, `deps_enforcement`, `max_wip`; `version`/`last_id` read-only) |
| `north skill install\|show\|check` | Install / print / version-check the embedded agent skill |
| `north tui` | Interactive terminal UI (human use only) |

Every task/board command — plus `config`, `skill check`, and `version` —
accepts `--plain` (tab-separated) or `--json` for stable, parseable output,
with structured JSON errors and one exit-code contract in every mode. `move`/`state`/`delete` accept comma-delimited id
batches (deduplicated, continue-on-error, per-id report). Full contract —
exit codes, plain columns, warning arrays — in
[docs/design/03_cli.md](docs/design/03_cli.md); `north completion <shell>`
generates shell completions.

---

## TUI

`north tui` opens a full-screen, keyboard-only terminal UI for humans: a
**board view** (draft column, the five status columns, archive column) and a
**list view** with a full-detail pane rendering the task body as Markdown.
Create and edit open your `$EDITOR` on the real task-file format; pickers
cover status, state, sort, and dependencies; `/` live-filters; `x` runs
doctor in place; `y` yanks the task id via OSC 52. Press **`?`** for the
complete key reference.

Colors come from three presets (`default` — inherits your terminal palette,
`saturated`, `high-contrast`) via `tui.theme` in the user-level
`~/.north/config.yml`, scaffolded on first run; `NO_COLOR` is honoured. Bad
config never blocks startup — it falls back to `default` with a warning.
Agents should use the CLI: the TUI needs a real TTY and produces no
machine-readable output.

## Dependencies
`depends_on` links tasks: a dependency is **resolved** once its task is
`done` or archived. Query with `north task list --deps met` ("what is
workable") / `--deps unmet` ("what is waiting" — the TUI's `!` tag, same
rule). Enforcement is one per-board setting,
`north config set deps_enforcement hint|validated|strict`: **hint** only
warns, **validated** (default) refuses dangling ids/self-refs/cycles and
heals dependents on delete, **strict** additionally refuses
`done`/`in_progress` moves while deps are unmet. Writes-only — levels switch
freely with no migration; details in
[docs/design/02_board-data-model.md](docs/design/02_board-data-model.md).

## Git
By default North does not commit — your task changes appear in `git status` and
you commit them with the rest of your work. Set `auto_commit: true` (via
`north config set auto_commit true`) to have North make a local commit per
change, using your system `git` — so linked worktrees, hooks, commit signing,
and your identity config all behave normally. It never pushes or pulls.

## Agents
North ships an installable **skill** that teaches agents the CLI — the
state/status model, the command surface, a typical work loop, and the
output/error contract:
```bash
north skill install                    # ./.claude/skills + ./.opencode/skills
north skill install --global           # ~/.claude/skills + ~/.config/opencode/skills
north skill check                      # is the installed skill this binary's version?
```
It works with Claude Code and opencode (and any agent that reads
`.claude/skills`). Agents claim work with `north take` — one atomic
select-and-claim under the board lock, so concurrent agents always get
different tasks — identified by `--assignee` or `$NORTH_AGENT`, optionally
capped by the `max_wip` board setting.

---

## License
[MIT](LICENSE.md)

## Development
```bash
make build         # go build -o bin/north ./cmd/north
make test          # go test ./...
make vet           # go vet ./... + gofmt check
make install       # go install ./cmd/north
```
All library code lives under `internal/` (the only installable package is
`cmd/north`); the design spec is in [docs/design/](docs/design/).
