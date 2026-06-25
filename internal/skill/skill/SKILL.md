---
name: north
description: >
  Manage project tasks with north, an in-repo Markdown task board CLI.
  Use when the user mentions tasks, the board, backlog, todos, project
  tracking, or wants to create, list, view, move, promote, archive, or
  otherwise manage work items. Also use for status updates and triage.
allowed-tools:
  - Bash(north *)
---

# north

`north` is an in-repo Markdown task board. Each task is a `.md` file with YAML
frontmatter, stored under a `north/` directory in the repo. The board is found
by walking up from the working directory (like `.git`), so commands work from
any subdirectory. There is no server — `north` operates directly on files.

Always start by running `north board` to see the current state. If it reports no
board, run `north init` first.

## Two axes: state and status

A task has two independent properties.

- **State** = where the task is in its lifecycle (its folder):
  `draft` → `active` → `archive`.
  - `draft` — captured but not yet on the active board (a human gate).
  - `active` — being worked.
  - `archive` — off the board, kept for history.
- **Status** = the workflow column, only meaningful while **active**:
  `ready → in_progress → done | failed | blocked`, and
  `done | failed | blocked → ready` for rework.

New tasks start as a **draft** with status `ready`. You must **promote** a draft
to active before you can change its status.

## Commands

Lifecycle (move between states):

```bash
north task create "<title>" [--agent A] [--labels a,b] [--depends-on task-3] [--body "..."]
north task promote <id>     # draft   → active
north task demote <id>      # active  → draft
north task archive <id>     # draft/active → archive
north task restore <id>     # archive → active
north task delete <id> -y   # remove permanently
```

Status (active tasks only):

```bash
north task move <id> <status>   # ready | in_progress | done | failed | blocked
```

Query:

```bash
north task list [--state draft|active|archive] [--status S] [--plain | --json]
north task view <id> [--plain | --json]
north board                     # counts per status (active) + draft/archive tally
north cleanup [--older-than DAYS]   # archive active 'done' tasks
```

## Rules for agents

- Run `north board` before acting to understand the current state.
- Use `--plain` (tab-separated) or `--json` for parseable output when listing or
  viewing; the default is human-formatted.
- A freshly created task is a **draft** — `promote` it before `move`-ing its
  status. `north task move` on a draft/archived task is rejected.
- `move` changes status in place (the file stays in `tasks/`). promote / demote /
  archive / restore move the file between folders and preserve status.
- Put descriptions, plans, blockers, and results in the task **body** — north
  does not impose body structure.
- Prefer driving the board through these commands rather than editing task files
  by hand, so ids, status, and timestamps stay consistent.
