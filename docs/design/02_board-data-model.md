# 2. Board data model

The board has exactly one object: the **task**. Each task is a Markdown file
named `task-<n>-<title-slug>.md`. A task has two orthogonal axes:

- **State** — its lifecycle *location*, i.e. the folder it lives in:
  `drafts/` (draft), `tasks/` (active), `archive/` (archive). State is the
  source of truth for "where" a task is.
- **Status** — its workflow *column*, stored only in frontmatter:
  `ready, in_progress, done, failed, blocked`. Status only changes while the
  task is **active**.

## Task schema (frontmatter)
```yaml
id: task-12              # task-<n>, unique across the board
title: Add login form
status: ready            # workflow status (frontmatter is the source of truth)
agent: opus4.8           # optional, free-form, opaque (executor/provider tag)
labels: [auth, backend]  # optional free-form tags
depends_on: [task-4]     # task ids
created_at: 2026-06-24T...
updated_at: 2026-06-24T...   # bumped on every mutation
```
State is **not** stored in frontmatter — it is the folder. The **body**
(everything after the frontmatter) is free text — description, plan, notes,
blockers, results — structured however the user/agent likes.

## State (lifecycle by folder)
```
drafts/  ──promote──▶  tasks/  ──archive──▶  archive/
   ▲                     │                      │
   └──────demote─────────┘                      │
   ▲                                             │
   └──────────────────restore───────────────────┘
   (also: drafts/ ──archive──▶ archive/)
```

| Verb | From → To | Command |
|---|---|---|
| promote | draft → active | `north task promote <id>` |
| demote | active → draft | `north task demote <id>` |
| archive | draft/active → archive | `north task archive <id>` |
| restore | archive → draft | `north task restore <id>` |

State moves relocate the file between folders and **preserve** status. New tasks
are created as **drafts** (status `ready`); promote them before working.

## Status (workflow, active-only)
```
ready ──▶ in_progress ──▶ done
              ├──▶ failed ──┐
              └──▶ blocked ─┘
   ▲                        │
   └────── reopen ──────────┘   (done/failed/blocked → ready)
```

| From | Allowed to |
|---|---|
| `ready` | `in_progress` |
| `in_progress` | `done`, `failed`, `blocked` |
| `done` / `failed` / `blocked` | `ready` (rework / reopen) |

`north task move <id> <status>` sets status. It rewrites frontmatter **in place**
(the file stays in `tasks/`) and is rejected unless the task is active and the
transition legal. Statuses and states are hardcoded for now; making them
configurable per board is future work.

## IDs
`task-<n>`, allocated as `max(existing) + 1` across every folder (drafts, tasks,
archive), so ids are never reused.
