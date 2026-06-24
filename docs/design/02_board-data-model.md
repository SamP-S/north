# 2. Board data model

The board has exactly one object: the **task**. Each task is a Markdown file
named `task-<n> - <Title-Slug>.md`, stored in the folder for its status.

## Task schema (frontmatter)
```yaml
id: task-12              # task-<n>, unique across the board
title: Add login form
status: ready            # mirrors the folder; the folder is the source of truth
agent: opus4.8           # optional, free-form, opaque (executor/provider tag)
labels: [auth, backend]  # optional free-form tags
depends_on: [task-4]     # task ids
created_at: 2026-06-24T...
updated_at: 2026-06-24T...   # bumped on every edit/move/archive
```
The **body** (everything after the frontmatter) is free text — description,
plan, notes, blockers, results — structured however the user/agent likes. North
imposes nothing on it.

## Lifecycle by folder
Status is represented by the folder the file lives in. Changing status moves the
file; the `status` frontmatter key is kept in sync as a readable mirror.

```
draft ──▶ ready ──▶ in_progress ──▶ done
            ▲              ├──▶ failed ──┐
            └──── resolve ─┴──▶ blocked ─┘
```

| From | Allowed to |
|---|---|
| `draft` | `ready` (the human gate) |
| `ready` | `in_progress` |
| `in_progress` | `done`, `failed`, `blocked` |
| `done` / `failed` / `blocked` | `ready` (rework / reopen) |

Illegal transitions are rejected. Statuses are hardcoded for now; making them
configurable per board is future work.

## Archive
`north/archive/` holds tasks taken off the active board. Archive is **orthogonal
to status**: an archived file keeps its last `status` in frontmatter and is
excluded from `north board` and the default `north task list` (use `--archived`).
`north task archive <id>` archives one; `north cleanup` archives done tasks.

## IDs
`task-<n>`, allocated as `max(existing) + 1` across every folder including
archive, so ids are never reused.
