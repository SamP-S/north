# North

[![CI](https://github.com/SamP-S/north/actions/workflows/ci.yml/badge.svg)](https://github.com/SamP-S/north/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE.md)

North is an **in-repo Markdown task board** with a CLI, modeled on
[Backlog.md](https://github.com/MrLesk/Backlog.md). The board lives in a `north/`
directory committed inside your own project repo — each task is a plain Markdown
file. There is no daemon and no central state: `north <cmd>` operates directly on
the files, and **git is entirely yours** (North never pushes or pulls).

---

## Requirements
- [Go](https://go.dev/dl/) 1.25+ (to build/install)

## Install
```bash
go install github.com/SamP-S/north/cmd/north@latest   # installs to $GOBIN / $GOPATH/bin
# or, from a clone:
make install          # go install ./cmd/north
```
This installs a single self-contained `north` binary — no runtime, no daemon.
`go install` places it in `$GOPATH/bin`; if `north` is then "command not
found", add that directory to your PATH (e.g. in your `~/.bashrc` / shell rc):

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

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
  drafts/            # state: draft
  tasks/             # state: active   (status in frontmatter)
  archive/           # state: archive
```
A task is one file, `<n>-<title-slug>.md`, in its state folder. `config.yml`
carries a `version: 1` format stamp; a board written by a newer North is
refused rather than misread. Creating a task without `--body` fills it from
`task-template.md` (Summary / Acceptance Criteria / Notes / Changes /
Comments) — edit or delete the file to change what new tasks start with;
North never parses it back. Mutating commands take a brief advisory lock
(`north/.lock`) so concurrent `north` calls cannot mint duplicate ids.

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
| `north task list [--state …] [--status S] [--assignee A] [--search TEXT] [--label L] [--sort id\|updated\|title\|assignee] [--reverse]` | List tasks (default active, newest first) |
| `north task view <id>` | Show a task |
| `north task edit <id> [--title --assignee --labels --depends-on --body \| --body-file \| --append-body]` | Edit a task |
| `north task move <id[,id…]> <status>` | Set status (any → any, in any state) |
| `north task state <id[,id…]> <draft\|active\|archive>` | Set lifecycle state (any → any) |
| `north task delete <id[,id…]> [-y]` | Delete tasks (`-y` required in machine/non-TTY modes and for batches) |
| `north board` | Active counts per status + draft/archive tally |
| `north cleanup [--older-than DAYS]` | Archive active done tasks |
| `north doctor [--fix]` | Board integrity check (duplicates, cycles, bad files, missing .gitattributes) |
| `north config list\|get\|set` | Read/write board settings (`auto_commit`; `version` is read-only) |
| `north skill install [--global] [--target claude\|opencode]` | Install the agent skill (default: both tools) |
| `north skill show` / `north skill check` | Print / version-check the embedded skill |
| `north tui` | Interactive terminal UI (human use only) |
| `north completion <shell>` | Shell completions (bash/zsh/fish/powershell) |
| `north version` | Print the north version |

Every task/board command accepts `--plain` (tab-separated) or `--json` for
stable, parseable output; ids are bare numbers (`north task view 12`).
`task list --plain` columns are `id  state  status  assignee  labels  title`.
`move`/`state`/`delete` accept a comma-delimited id batch — deduplicated,
continue-on-error, with a per-id report. Failures print
`error [<code>]: <message>` to stderr (with `--json`, a
`{"error":{"code":…,"message":…}}` object instead), and exit codes follow one
contract in every mode: 0 success, 1 internal, 2 invalid/usage, 3 not_found,
4 conflict (a partially failed batch exits with the shared failure code).
`NO_COLOR` is honoured in the TUI.

---

## TUI

`north tui` opens a full-screen interactive terminal UI:

- **Board view** — the whole two-axis model on one screen: a `draft` column on the left, the status columns (`ready | in_progress | blocked | done | failed`) for active tasks, and an `archive` column on the right; columns sort newest-first by default. Cards in the two state columns carry a status-colored dot, and all cards show dim tags: `@` assignee set, `!` waiting on an unmet dependency (resolves when the dependency is done or archived), `&` other tasks depend on it.
- **List view** — all tasks sorted newest-first in a scrollable list; right pane shows the selected task in full detail (id, deps with their status, rendered Markdown body).
- **Tab** switches between the two views; **Enter** on a board card opens the task in a scrollable popup (`e` edits from there, esc closes).
- **`c`** creates and **`e`** edits a task in `$VISUAL`/`$EDITOR` — the buffer is the real task-file format (frontmatter + body); quitting the editor with a non-zero exit (`:cq`) cancels.
- **`m`** opens a status picker; **`s`** opens a state picker (draft/active/archive); **`d`** deletes (with confirm).
- **`/`** live-filters **both views** in place (id, title, assignee, labels, body — case-insensitive) — the board narrows its columns, the list its rows; **esc** clears the filter.
- A status bar above the footer confirms every action (green), warns (yellow — e.g. setting status on a draft), and reports errors (red).
- **`o`** opens a sort picker (id / updated / title / assignee, each ascending or descending; default id ↓); **`g`/`G`** jump to top/bottom; **`r`** reloads from disk; **`?`** shows the full key reference.

The TUI is keyboard-only by design (no mouse) and for human use. Agents should use the CLI commands — the TUI requires a real TTY and produces no machine-readable output.

---

## Git
By default North does not commit — your task changes appear in `git status` and
you commit them with the rest of your work. Set `auto_commit: true` (via
`north config set auto_commit true`) to have North make a local commit per
change, using your system `git` — so linked worktrees, hooks, commit signing,
and your identity config all behave normally. It never pushes or pulls.

## Agents
North ships an installable **skill** that teaches agents the CLI:
```bash
north skill install                    # ./.claude/skills + ./.opencode/skills
north skill install --target claude    # just one tool (claude|opencode)
north skill install --global           # ~/.claude/skills + ~/.config/opencode/skills
north skill check                      # is the installed skill this binary's version?
```
The skill describes the state/status model, the commands, a typical work loop,
and the output/error contract. It works with Claude Code and opencode (and any
agent that reads `.claude/skills`).

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

## Repository layout
```
north/
  cmd/north/         # main package — the only installable binary
  internal/
    errors/          # BoardError (NotFound / Conflict / Invalid)
    models/          # Task + the state/status value sets
    board/           # discovery, scaffolding, config, ids
    tasks/           # task ops, frontmatter round-trip, snapshot, doctor
    git/             # optional local auto-commit (exec system git)
    render/          # human / --plain / --json output
    skill/           # embedded agent skill + installer
    tui/             # interactive terminal UI (bubbletea)
    cli/             # the `north` cobra command tree
    version/         # build-time version string
  docs/design/       # design spec
```
