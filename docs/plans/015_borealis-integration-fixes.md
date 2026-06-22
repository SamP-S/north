# 015 — Borealis Integration Fixes

## Summary

Fix loose ends left by the 013/014 nested-model and API refactors. These are all
breaks in the Aurora↔Borealis contract or dead references to the old flat model:

1. **`startup.py` crash** — `_reset_in_progress_tasks` iterates `state.tasks`,
   which no longer exists on `BoardState`. Rewrite to traverse
   `projects → features → tasks`.
2. **Aurora client route drift** — `aurora/service/borealis_client.py` calls
   `/api/tasks/{project}/{feature}/{task_id}` and `/api/features/{project}/...`,
   but Borealis serves `/api/projects/{project}/features/{feature}/tasks/{task_id}`
   and `/api/projects/{project}/features/{feature}`. Update all client methods
   (`get_task`, `update_task_status`, `update_feature_status`, `requeue_feature`)
   to the real routes.
3. **`review.py` feature lookup hack** — `approve/rollback/reject_feature` fetch
   feature data via `client.get_task(project, feature_id, "_feature")`. Add a
   proper `BorealisClient.get_feature(project, feature)` using
   `GET /api/projects/{project}/features/{feature}` (added in 014) and use it.
4. **Dependency-aware queue** — `resolver.resolve_eligible_tasks` (task DAG +
   feature dependency gating) exists but is unused; `/api/queue` returns all
   queued tasks sorted by `ready_at` only, and Aurora takes `tasks[0]`. Change
   `GET /api/queue` to return eligible tasks via `resolve_eligible_tasks`
   ordering, plus in-progress tasks (still needed for CLI display). Eligible
   ordering applies to queued tasks only.

Out of scope: the `task_state.__dict__["pipeline_def"]` smuggling and
`board_path` fallback in Aurora's `TaskRunner` (Aurora-internal; deferred to the
architecture rework under discussion), and the status-model simplification
(separate redesign, see memory/backlog).

## Files to Modify

- `borealis/borealis/service/startup.py` — nested traversal in
  `_reset_in_progress_tasks`
- `borealis/borealis/service/main.py` — `/api/queue` uses
  `resolve_eligible_tasks` for queued ordering
- `aurora/aurora/service/borealis_client.py` — fix all route paths; add
  `get_feature`
- `aurora/aurora/service/review.py` — use `get_feature` instead of the
  `_feature` task hack
- `borealis/tests/test_api.py` — queue eligibility/order tests
- `borealis/tests/test_resolver.py` — keep passing (no behavior change expected)
- `aurora/tests/test_review.py`, `aurora/tests/integration/*` — update mocked
  routes to the real Borealis paths

## Todo

- [x] 1. Fix `_reset_in_progress_tasks` to traverse the nested model; add a
      startup test covering reset-on-restart
- [x] 2. Fix `BorealisClient` route paths; add `get_feature`
- [x] 3. Switch `review.py` to `get_feature`
- [x] 4. Make `/api/queue` dependency-aware via `resolve_eligible_tasks`
      (queued tasks in eligible order + in-progress tasks)
- [x] 5. Update/extend tests (borealis API, aurora review, integration mocks);
      run full suite + ruff

## Change History

- [2026-06-11] Plan created
- [2026-06-11] Implemented all items. `startup.py` now traverses the nested
  model (new `borealis/tests/test_startup.py`, 2 tests). `/api/queue` returns
  in-progress tasks (ready_at order) followed by resolver-eligible queued
  tasks; 4 new queue tests in `test_api.py`. `BorealisClient` routes corrected
  to `/api/projects/{project}/features/...`; new `get_feature`; `review.py`
  uses it instead of the `get_task(..., "_feature")` hack (test mock updated).
  Integration tests contained no mocked Borealis routes — no changes needed
  there. Also fixed the pre-existing `startup.py` E501. 124/124 tests passing,
  ruff clean (ruff --fix also removed pre-existing trailing whitespace across
  aurora test files).
