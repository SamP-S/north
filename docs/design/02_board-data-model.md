# 2. Board data model

The board has exactly one object: the **task**. Each task is a Markdown file
named `<n>-<title-slug>.md` (e.g. `12-add-login.md`). A task has two orthogonal
axes:

- **State** — its lifecycle *location*, i.e. the folder it lives in:
  `drafts/` (draft), `tasks/` (active), `archive/` (archive). State is the
  source of truth for "where" a task is.
- **Status** — its workflow *column*, stored only in frontmatter:
  `ready, in_progress, done, failed, blocked`. Status only changes while the
  task is **active**.

Both axes are **freeform**: any state can move to any other state, and any
status to any other status, in a single step. North validates the *value*
(unknown states/statuses are rejected) but does not prescribe a path — the
user is allowed to use the board how they want.

## Task schema (frontmatter)
```yaml
id: "12"                 # bare number, unique across the board, quoted string
title: Add login form
status: ready            # workflow status (frontmatter is the source of truth)
agent: opus4.8           # optional, free-form, opaque (executor/provider tag)
labels: [auth, backend]  # optional free-form tags
depends_on: ["4"]        # task ids
created_at: "2026-06-24T…"
updated_at: "2026-06-24T…"   # bumped on every mutation
```
State is **not** stored in frontmatter — it is the folder. The **body**
(everything after the frontmatter) is free text — description, plan, notes,
blockers, results — structured however the user/agent likes.

**Unknown frontmatter keys round-trip.** North parses frontmatter as a YAML
node tree and only overlays the keys it owns, so custom keys a user adds by
hand (e.g. `priority: high`) survive every North rewrite, in their original
position. CRLF line endings are tolerated on read; files are always written
with LF, atomically (temp file + rename).

## State (lifecycle by folder)

`north task state <id> <draft|active|archive>` moves the file between state
folders and **preserves status**. Any state → any other state is legal,
including `archive → active` directly. New tasks are created as **drafts**
(status `ready`).

| State | Folder | Meaning |
|---|---|---|
| `draft` | `drafts/` | captured, not yet on the board (human gate) |
| `active` | `tasks/` | on the board, being worked |
| `archive` | `archive/` | off the board, kept for history |

## Status (workflow, active-only)

`north task move <id> <status>` sets status. It rewrites frontmatter **in
place** (the file stays in `tasks/`) and is rejected unless the task is
active. Any status → any other status is legal (e.g. `ready → failed`).

| Status | Meaning |
|---|---|
| `ready` | available to be picked up |
| `in_progress` | actively being worked |
| `done` | finished successfully |
| `failed` | attempted but did not work out |
| `blocked` | cannot proceed; body should say what unblocks it |

The status *list* is hardcoded for now; making it configurable per board is
future work.

## IDs

Bare numbers (`"12"`), allocated as `max(existing) + 1` across every folder
(drafts, tasks, archive), so ids are never reused while a task exists. Stored
as quoted YAML strings so they never degrade to integers. Duplicate ids (e.g.
after a git merge where two branches each created the same id) are detected on
every load (warning) and repaired by `north doctor --fix`.

## Tolerant loading

All read paths (list, board, TUI, lookups) parse the whole board through one
tolerant snapshot: a malformed task file becomes a *warning* naming the file
(stderr, or `"warnings"` in `--json`) instead of an error — one bad file never
takes down the board. `north doctor` reports (and `--fix` repairs where safe)
malformed files, duplicate ids, filename/id drift, dangling `depends_on`,
dependency cycles, and CRLF files.
