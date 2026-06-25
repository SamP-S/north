# North

North is an **in-repo Markdown task board** with a CLI and an optional MCP
server, modeled on [Backlog.md](https://github.com/MrLesk/Backlog.md). The board
lives in a `north/` directory committed inside your own project repo — each task
is a plain Markdown file. There is no daemon and no central state: `north <cmd>`
operates directly on the files, and **git is entirely yours** (North never
pushes or pulls).

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
north init                                  # create the north/ board + AGENTS.md
north task create "Add login form" --agent opus4.8 --labels auth
north task move task-1 ready                # draft -> ready (the human gate)
north task move task-1 in_progress
north task view task-1
north task move task-1 done
north board                                 # counts per status
```

---

## The board
`north init` scaffolds, inside your repo:
```
north/
  config.yml                              # board marker + settings
  draft/ ready/ in_progress/ done/ failed/ blocked/
  archive/
```
A task is one file, `task-<n> - <Title-Slug>.md`, in the folder for its status.

### Task file
```yaml
---
id: task-12
title: Add login form
status: ready            # mirrors the folder (the folder is the source of truth)
agent: opus4.8           # optional, free-form, opaque executor/provider tag
labels: [auth]           # optional free-form tags
depends_on: [task-4]     # task ids
created_at: 2026-06-24T...
updated_at: 2026-06-24T...
---
Free-form body: description, plan, notes, blockers, results — your structure.
```

### Lifecycle
`draft → ready → in_progress → done | failed | blocked`, with
`failed/blocked/done → ready` for rework. Status is the folder; `north task move`
validates the transition and moves the file. Statuses are fixed for now
(configurable per board is future work).

### Archive
`north task archive <id>` (or `north cleanup` for done tasks) moves files into
`north/archive/`, off the active board. Archived tasks are hidden from `north
board` and `north task list` unless you pass `--archived`.

---

## CLI
| Command | Description |
|---|---|
| `north init` | Scaffold the board + `AGENTS.md` |
| `north task create <title> [--agent --labels --depends-on --body \| --body-file]` | Create a task (draft) |
| `north task list [--status S] [--archived] [--plain \| --json]` | List/filter tasks |
| `north task view <id> [--plain \| --json]` | Show a task |
| `north task edit <id> [--title --agent --labels --depends-on --body \| --body-file]` | Edit a task |
| `north task move <id> <status>` | Change status |
| `north task archive <id>` | Archive a task |
| `north task delete <id> [-y]` | Delete a task |
| `north board` | Counts per status |
| `north cleanup [--older-than DAYS]` | Archive done tasks |
| `north mcp start \| stop \| status \| run` | Manage the optional MCP server |

`--plain` and `--json` give agents and scripts stable, parseable output.

---

## Git
By default North does not commit — your task changes appear in `git status` and
you commit them with the rest of your work. Set `auto_commit: true` in
`north/config.yml` to have North make a local commit per change. It never pushes
or pulls.

## MCP (optional, for agents)
```bash
north mcp start    # serves http://127.0.0.1:8001/mcp (port from config.yml)
north mcp stop
```
A single MCP endpoint exposing the task tools (`list_tasks`, `get_task`,
`create_task`, `set_task_status`, `edit_task`). Optional bearer token via the
`MCP_TOKEN` env var; the server binds loopback only.

---

## Development
```bash
make build         # go build -o north ./cmd/north
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
    models/          # Task + status state machine
    board/           # discovery, scaffolding, config, ids
    tasks/           # task CRUD + frontmatter read/write
    git/             # optional local auto-commit (go-git)
    instructions/    # AGENTS.md text
    render/          # human / --plain / --json output
    cli/             # the `north` cobra command tree
    service/         # the optional MCP server (net/http + mcp-go)
  docs/design/       # design spec
```
