# North

North is an **in-repo Markdown task board** with a CLI and an optional MCP
server, modeled on [Backlog.md](https://github.com/MrLesk/Backlog.md). The board
lives in a `north/` directory committed inside your own project repo — each task
is a plain Markdown file. There is no daemon and no central state: `north <cmd>`
operates directly on the files, and **git is entirely yours** (North never
pushes or pulls).

---

## Requirements
- Python 3.12+
- [uv](https://docs.astral.sh/uv/)

## Install
```bash
bash scripts/install.sh        # or: uv tool install .
```
This puts `north` on your PATH. Nothing else to provision.

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
| `north instructions` | Print agent guidance (same as `AGENTS.md`) |
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
uv sync --all-extras
scripts/install-dev.sh         # put the editable `north` on your PATH (dev only)
uv run ruff check .
uv run mypy north
uv run pytest
```

## Repository layout
```
north/
  north/
    core/          # the board: discovery, tasks, config, optional git commit
    service/       # the optional MCP server (FastAPI + FastMCP)
    cli/           # the `north` CLI
  tests/
  scripts/install.sh
  docs/design/     # design spec
```
