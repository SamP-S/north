# 019 — Test Suite Repair and Review Worktree Fix

## Summary

Fix the five pre-existing Aurora test failures surfaced during plan 017
verification. They fall into two groups:

1. **Real product bug** — `reject_feature` and `rollback_feature`
   (`aurora/service/review.py`) reset the feature branch with
   `git branch -f`, which git refuses (exit 128) when the branch is checked
   out in a worktree. In production the feature branch always has a worktree,
   so both operations crash. Fails today in
   `test_review.py::test_reject_resets_and_closes`; latent in rollback (its
   test just doesn't create a worktree).
2. **Stale tests from the pre-extraction monolith** — three integration test
   files still target the old `TaskRunner(aurora_path, board_path,
   aurora_home)` API, the removed `TaskModel`, direct board-file writes
   (`update_task_frontmatter`), and the removed
   `_maybe_mark_feature_review`. Post plan 009/015, `TaskRunner` takes a task
   dict and reports via `BorealisClient.update_task_status`; board writes and
   feature auto-promotion live in Borealis.

Decisions (confirmed 2026-06-11):

- Rewrite `test_full_task_run.py` and `test_pause_resume.py` against the
  current API (mocked `BorealisClient`) — keep the orchestration coverage.
- Delete `test_feature_lifecycle.py` — fully superseded by Borealis's
  `test_api.py::test_patch_task_done_promotes_feature_to_review`.
- Rollback resets the branch *inside the worktree* (`git reset --hard`) when
  one exists, keeping it usable for the requeued tasks; reject removes the
  worktree first, then `branch -f` in the managed clone.

## Files to Modify

- `aurora/aurora/service/review.py` — worktree-aware branch reset in
  `rollback_feature` and `reject_feature`
- `aurora/tests/test_review.py` — add rollback-with-worktree regression test
- `aurora/tests/integration/test_full_task_run.py` — rewrite against
  `run_task(task_dict)` + mocked `BorealisClient`
- `aurora/tests/integration/test_pause_resume.py` — same
- `aurora/tests/integration/test_feature_lifecycle.py` — delete

## Todo

- [x] 1. `review.py::reject_feature` — call
      `remove_worktree(worktree_path_for(project, feature_id))` *before*
      `git branch -f`; keep the existing call order otherwise (push, Borealis
      update last)
- [x] 2. `review.py::rollback_feature` — if the feature worktree exists, run
      `git reset --hard <merge-base>` inside the worktree (updates the
      checked-out branch in place); otherwise `git branch -f` in the managed
      clone as today; push `--force-with-lease` unchanged
- [x] 3. `test_review.py` — new test: rollback with an existing worktree
      resets the branch and leaves the worktree present; confirm
      `test_reject_resets_and_closes` now passes unchanged
- [x] 4. Rewrite `tests/integration/test_full_task_run.py` — build a task
      dict (`task_id`, `project`, `feature`, `pipeline`, `branch`, `body`,
      `task_path`), monkeypatch `create_worktree`, `load_agent_definitions`,
      `prepare_agent`, `run_pipeline` (as today) plus `BorealisClient`;
      assert `run_task` returns `DONE` and `update_task_status` was awaited
      with `done` and result content containing the pipeline output
- [x] 5. Rewrite `tests/integration/test_pause_resume.py` — same harness;
      scenario 1: `prepare_agent` raises `AgentPrepareError` → `BLOCKED`
      reported, pipeline never called; scenario 2: pipeline returns `DONE` →
      status always reported to Borealis
- [x] 6. Delete `tests/integration/test_feature_lifecycle.py` (behaviour
      covered by `borealis/tests/test_api.py`)
- [x] 7. Full suite: `uv run --extra dev pytest tests` in `aurora/` (expect 0
      failures, 0 collection errors), `ruff check`, `mypy aurora/` (no new
      errors beyond the 14 pre-existing in `borealis_client.py`/`review.py`)

## Change History

- [2026-06-11] Plan created from plan 017 verification findings.
- [2026-06-11] All items implemented. `reject_feature` removes the worktree
  before `branch -f`; `rollback_feature` reuses `reset_worktree()` (existing
  helper in `git/worktree.py`) when the worktree exists, falling back to
  `branch -f` otherwise. Added `test_rollback_with_worktree_resets_in_place`;
  rewrote `test_full_task_run.py` (queued→done reported to Borealis;
  missing-pipeline→blocked) and `test_pause_resume.py` (prepare failure
  blocks before pipeline; final status always reported) against the current
  `run_task(task_dict)` + mocked `BorealisClient` API; deleted
  `test_feature_lifecycle.py` (covered by Borealis `test_api.py`). Result:
  aurora 86/86 passed (was 70 passed + 3 failed + 2 collection errors),
  borealis 63/63, ruff clean, mypy unchanged (14 pre-existing errors in
  `borealis_client.py`/`review.py`).
