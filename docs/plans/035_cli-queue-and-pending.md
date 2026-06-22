# 035 — North CLI: add `queue` command + `conversation list --pending`

## Context

A gap pass over the unified north CLI vs. the Borealis/Aurora REST surface found
two endpoints with no CLI command:
- `GET /api/queue` — the active (in-progress + eligible queued) task list. The old
  `borealis queue` command was dropped in the unified-CLI rewrite and never
  reintroduced.
- `GET /api/conversations/pending` — the cross-project decomposition queue.

This plan exposes both, per the user's chosen shapes:
- a new top-level **`north queue [--project <name>]`** observation command;
- **`conversation list`** gains a **`--pending`** filter (not a separate command),
  and its `project` positional becomes optional so the pending queue can be
  viewed cross-project.

## Changes

- `north/north/cli/commands/observe.py` — add `queue(args, ctx)` → `GET /api/queue`
  with optional `project` param; print `task_id [status] project/feature title`,
  or "queue is empty".
- `north/north/cli/commands/conversation.py` — `list_` learns `--pending`: when set,
  hit `GET /api/conversations/pending` (optional `project` param) and print
  `id status project title`; otherwise require `project` and keep current behaviour.
- `north/north/cli/main.py`:
  - wire `north queue` in `_add_observe_control` (alongside logs/pause/resume).
  - `conversation list`: `project` → `nargs="?"`, add `--pending`.
- Tests: `north/tests/test_cli_observe.py` (queue: rows + empty + project filter),
  `north/tests/test_cli_conversation.py` (pending filter; project-required error
  when neither given).

## TODO

- [x] 1. `queue` command + parser.
- [x] 2. `conversation list --pending` + optional project + parser.
- [x] 3. Tests for both.
- [x] 4. `uv run ruff check north` clean; `uv run pytest north/tests` green (92 passed); `--help` smoked.

## Change history

- [2026-06-16] Plan written. Closes the two orphaned endpoints found in the gap pass.
- [2026-06-16] Implemented `north queue` (observe.py) and `conversation list --pending`
  (optional project positional). Added 5 tests. 92 north tests pass, ruff clean.
