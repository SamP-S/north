# 2. Board data model

The board has exactly one object: the **task**. Each task is a Markdown file
named `<n>-<title-slug>.md` (e.g. `12-add-login.md`). A task has two orthogonal
axes:

- **State** — its lifecycle *location*, i.e. the folder it lives in:
  `drafts/` (draft), `tasks/` (active), `archive/` (archive). State is the
  source of truth for "where" a task is.
- **Status** — its workflow *column*, stored only in frontmatter:
  `ready, in_progress, blocked, done, failed`. Status can change in any
  state but is only shown on the board while the task is **active**.

Both axes allow **free movement within a fixed value set**: any state can
move to any other state, and any status to any other status, in a single
step. North validates the *value* (unknown states/statuses are rejected —
hand-edited ones surface via warnings and `north doctor`) but does not
prescribe a path — the user is allowed to use the board how they want.

## Task schema (frontmatter)
```yaml
id: "12"                 # bare number, unique across the board, quoted string
title: Add login form
status: ready            # workflow status (frontmatter is the source of truth)
assignee: claude:opus    # optional, free-form — a person ("john") or an agent
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

## Status (workflow)

`north task move <id> <status>` sets status. It rewrites frontmatter **in
place** (the file stays in its state folder) and works in **any state** —
though status is only *visible* on the board while the task is active, so a
warning is printed when moving a draft/archive task. Any status → any other
status is legal (e.g. `ready → failed`).

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
as quoted YAML strings so they never degrade to integers. Mutating commands
serialise through a brief advisory file lock (`north/.lock`, created
`O_CREATE|O_EXCL`, stolen when stale), so concurrent `north` processes cannot
mint the same id. Duplicate ids can still arrive from outside (a git merge
where two branches each created the same id): they are detected on every load
(warning) and repaired by `north doctor --fix`.

## Board files

Besides the state folders, `init` scaffolds three board-owned files (each
written only when missing, so user edits survive re-init):

- `config.yml` — discovery marker + settings, stamped `version: 1` (see
  [05_configuration.md](05_configuration.md)). A board with a newer stamp is
  refused on load ("created by a newer north") rather than misread.
- `task-template.md` — the body scaffold bodyless creates are filled from
  (Summary / Acceptance Criteria / Notes / Changes / Comments). A suggestion,
  not schema: the user may edit or delete it (missing/empty ⇒ blank bodies),
  and North never parses it back.
- `.gitattributes` (`* text eol=lf`) — keeps board files LF on every clone.
  `north doctor` warns when it is missing; `--fix` restores it.

## Dependencies

`depends_on` holds task ids. A dependency is **resolved** when its task is
`done` or in `archive/` (terminal ≈ done) — one definition shared by the
CLI's `--deps met|unmet` filter, write-side enforcement, and the TUI's `!`
tag. An id that resolves to no task (or a cycle member) reads as unmet
forever until repaired.

How strictly the links are enforced is the `deps_enforcement` config key
(hint | validated | strict — see [05_configuration.md](05_configuration.md)).
Enforcement gates **writes only** — it never rewrites stored data on load, so
switching levels needs no migration and merged-in damage stays doctor's
domain:

| Event | hint | validated (default) | strict |
|---|---|---|---|
| dangling id on create/edit | warn (forward refs allowed) | refuse | refuse |
| self-reference / cycle on edit | warn | refuse | refuse |
| `move done`/`in_progress`, deps unmet | warn | warn | refuse |
| `state archive`, deps unmet | allow | allow | allow (terminal = abandon) |
| delete with dependents | warn, refs left dangling | heal: id dropped from dependents | heal |

Delete-healing runs under the same lock hold and bumps the dependents'
`updated_at`. `north doctor` flags dangling refs and cycles at every level;
`--fix` removes dangling refs (including deliberate forward references —
running `--fix` is an explicit ask).

## Tolerant loading

All read paths (list, board, TUI, lookups) parse the whole board through one
tolerant snapshot: a malformed task file becomes a *warning* naming the file
(stderr, or `"warnings"` in `--json`) instead of an error — one bad file never
takes down the board. `north doctor` reports (and `--fix` repairs where safe)
malformed files, duplicate ids, filename/id drift, dangling `depends_on`,
dependency cycles, CRLF files, and a missing `.gitattributes`.
