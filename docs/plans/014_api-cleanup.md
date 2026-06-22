# 014 — API Cleanup

## Summary

Small consistency/cleanup pass on the Borealis API following the 013 nested-model
refactor:

1. Add `GET /api/projects/{project}/features/{feature}` (single-feature lookup,
   matching the task get-by-id pattern).
2. Move `list_features` from `main.py` into `features.py` (route ownership
   consistency).
3. Hoist inline imports (`HTTPException`, `TaskStatus`, `FeatureStatus`) in
   `main.py` to module-level.
4. Replace the `_supervisor._board_state = _board_state` private-attribute
   reach-around in `main.py` with a constructor param on `Supervisor`.
5. SSE event queue removed entirely (dead code, no publisher); documented as a
   planned feature in `docs/design/99_planned-features.md` for re-add once the
   frontend needs live updates.
6. Unify `update_task`/`update_feature` (PUT) to use frontmatter writer helpers
   instead of manual `frontmatter.load`/`write_text`, matching the `/status`
   PATCH handlers' use of `update_task_frontmatter`/`update_feature_frontmatter`.
7. (no change) No project-update endpoint — expected behaviour, register/
   unregister only.

## Files to Modify

- `borealis/service/board/writer.py` — extend `_update_frontmatter` (or add a
  sibling helper) to optionally replace the document body
- `borealis/service/api/features.py` — add `get_feature`, move `list_features`
  here, use writer helper in `update_feature`
- `borealis/service/api/tasks.py` — use writer helper in `update_task`
- `borealis/service/main.py` — remove `list_features`, hoist imports, drop
  `_board_state` reach-around
- `borealis/service/orchestrator/supervisor.py` — accept `initial_state` in
  `__init__`
- `borealis/tests/test_api.py` — add test for new `GET` feature endpoint

## Todo

- [x] 1. Extend `writer.py` with a body-replacing frontmatter update helper
- [x] 2. Add `GET /api/projects/{project}/features/{feature}`; move `list_features`
      into `features.py`
- [x] 3. Hoist inline imports in `main.py`; remove `_board_state` reach-around via
      `Supervisor.__init__(initial_state=...)`
- [x] 4. Refactor `update_task`/`update_feature` PUT handlers to use the new
      writer helper
- [x] 5. Update/add tests; run full suite + ruff

## Change History

- [2026-06-10] Plan created
- [2026-06-10] Implemented all items. Added `replace_task_file`/`replace_feature_file`
  to `writer.py` (both delegate to `_update_frontmatter` with an optional `body`
  param). Added `GET /api/projects/{project}/features/{feature}` and moved
  `list_features` into `features.py`. Hoisted `HTTPException`/`TaskStatus`/
  `FeatureStatus`/`json` imports to module level in `main.py`; `Supervisor.__init__`
  now takes `initial_state`, removing the `_board_state` reach-around. `update_task`/
  `update_feature` PUT handlers now use the new writer helpers instead of manual
  frontmatter load/dump. Added 3 new tests for the feature-get endpoint. 51/51
  tests passing, ruff clean (excluding pre-existing unrelated `startup.py:21` E501).
- [2026-06-10] Removed SSE entirely: `GET /api/events`, `git_watcher.sse_event_queue`,
  CLI `borealis logs` command, and `BorealisClient.sse_stream`. Documented as a
  planned re-add in `docs/design/99_planned-features.md` with a concrete event
  taxonomy (board reload, task/feature status changes, queue activity). 51/51
  tests passing, ruff clean.
