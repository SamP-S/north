# North

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
This puts a single self-contained `north` binary on your PATH. Nothing else to
provision — no runtime, no daemon.

## Quick start
```bash
cd your-project
north init                                  # create the north/ board
north task create "Add login form" --agent opus4.8 --labels auth   # lands in drafts/
north task promote task-1                   # drafts -> tasks (onto the active board)
north task move task-1 in_progress          # change status (active tasks only)
north task view task-1
north task move task-1 done
north board                                 # active counts per status
```

---

## Two axes: state and status
A task has two independent properties:

- **State** — its lifecycle *location* (the folder): `draft` → `active` →
  `archive`. Changed with `promote` / `demote` / `archive` / `restore`.
- **Status** — its workflow *column* (frontmatter, active-only):
  `ready → in_progress → done | failed | blocked`, with
  `done/failed/blocked → ready` for rework. Changed with `north task move`.

New tasks start as a **draft** (status `ready`); `promote` them before changing
status. State moves relocate the file and preserve status; `move` rewrites status
in place.

## The board
`north init` scaffolds, inside your repo:
```
north/
  config.yml         # board marker + settings (auto_commit)
  drafts/            # state: draft
  tasks/             # state: active   (status in frontmatter)
  archive/           # state: archive
```
A task is one file, `task-<n> - <Title-Slug>.md`, in its state folder.

### Task file
```yaml
---
id: task-12
title: Add login form
status: ready            # workflow status (frontmatter is the source of truth)
agent: opus4.8           # optional, free-form, opaque executor/provider tag
labels: [auth]           # optional free-form tags
depends_on: [task-4]     # task ids
created_at: 2026-06-24T...
updated_at: 2026-06-24T...
---
Free-form body: description, plan, notes, blockers, results — your structure.
```

---

## CLI
| Command | Description |
|---|---|
| `north init` | Scaffold the board (`drafts/ tasks/ archive/`) |
| `north task create <title> [--agent --labels --depends-on --body \| --body-file]` | Create a task (drafts/) |
| `north task list [--state draft\|active\|archive\|all] [--status S] [--plain \| --json]` | List tasks (default active) |
| `north task view <id> [--plain \| --json]` | Show a task |
| `north task edit <id> [--title --agent --labels --depends-on --body \| --body-file]` | Edit a task |
| `north task move <id> <status>` | Set status (active tasks only) |
| `north task promote \| demote \| archive <id>` | Change state |
| `north task restore <id>` | Restore from archive → drafts (for review) |
| `north task delete <id> [-y]` | Delete a task |
| `north board` | Active counts per status + draft/archive tally |
| `north cleanup [--older-than DAYS]` | Archive active done tasks |
| `north skill install [--global]` | Install the agent skill (Claude Code + opencode) |
| `north skill show` | Print the embedded skill |
| `north tui` | Interactive terminal UI (human use only) |
| `north version` | Print the north version |

`--plain` and `--json` give agents and scripts stable, parseable output.

---

## TUI

`north tui` opens a full-screen interactive terminal UI:

- **Board view** — kanban columns (`ready | in_progress | done | failed | blocked`) for active tasks, with draft/archive counts in the footer.
- **List view** — all tasks sorted newest-first in a scrollable list; right pane shows the selected task in full detail.
- **Tab** switches between the two views; **Enter** on a board card jumps to its detail.
- **`e`** opens the selected task in `$EDITOR`; **`c`** creates a new task the same way.
- **`m`** opens a status picker (active tasks); **`p`** promotes/demotes; **`a`** archives or restores; **`d`** deletes.
- **`/`** live-filters the task list; **`?`** shows the full key reference.

The TUI is for human use. Agents should use the CLI commands — the TUI requires a real TTY and produces no machine-readable output.

---

## Git
By default North does not commit — your task changes appear in `git status` and
you commit them with the rest of your work. Set `auto_commit: true` in
`north/config.yml` to have North make a local commit per change. It never pushes
or pulls.

## Agents
North ships an installable **skill** that teaches agents the CLI:
```bash
north skill install            # ./.claude/skills + ./.opencode/skills
north skill install --global   # ~/.claude/skills + ~/.config/opencode/skills
```
The skill describes the state/status model and the commands. It works with Claude
Code and opencode (and any agent that reads `.claude/skills`).

---

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
    models/          # Task + state & status machines
    board/           # discovery, scaffolding, config, ids
    tasks/           # task ops + frontmatter read/write
    git/             # optional local auto-commit (go-git)
    render/          # human / --plain / --json output
    skill/           # embedded agent skill + installer
    tui/             # interactive terminal UI (bubbletea)
    cli/             # the `north` cobra command tree
  docs/design/       # design spec
```
