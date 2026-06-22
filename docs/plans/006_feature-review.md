# 006 — Feature Review Flow

> **DEPRECATED** [2026-06-11] — Never implemented and now stale: file paths
> target the pre-extraction monolith (superseded by the Aurora/Borealis split
> in plan 009), Telegram items were already deferred to
> `docs/design/99_planned-features.md`, and item 3 (auto-promote to `review`)
> was implemented independently in `borealis/service/api/tasks.py`. The
> remaining approve/rollback/reject flow should be re-planned against the
> current layout and the v2 architecture (`docs/design/01_v2-architecture.md`).

## Summary

Implement the full feature lifecycle gate: runner sets `status: review` when all tasks are done; `approve`, `rollback`, and `reject` operations with board archiving and worktree removal. Telegram notifications are deferred — see `docs/design/99_planned-features.md` § Telegram Notifications. Depends on 005 (git layer) and 004 (task runner). Note: files paths below reflect the pre-extraction monolith; actual targets are `borealis/` and `aurora/` after plan 009.

## Files to Create / Modify

```
service/
  notifications.py             # Telegram sender: dedup, retry, all event types
  review.py                    # approve / rollback / reject logic
  api/
    features.py                # POST /api/features/{project}/{feature}/{approve,rollback,reject}
  orchestrator/
    task_runner.py             # extend update_board: set feature status: review when all tasks done
tests/
  test_notifications.py
  test_review.py
```

## Todo

- [~] 1. ~~Telegram sender~~ — deferred; see `docs/design/99_planned-features.md` § Telegram Notifications
- [~] 2. ~~Telegram notification events~~ — deferred; see `docs/design/99_planned-features.md` § Telegram Notifications
- [ ] 3. `borealis/service/api/tasks.py` — feature auto-promoted to `review` when all tasks done (already implemented in tasks.py:80-97); verify behaviour is correct
- [ ] 4. `service/review.py` — `approve_feature(project, feature_id, board_path, aurora_home)`: merge feature branch into `base_branch` in managed clone (`git merge --no-ff`); on conflict: abort (`git merge --abort`), return `409` with conflict details, feature remains `review`; on success: push `base_branch` to origin; set feature `status: merged`; archive board (`active/{feature}/ → archived/{feature}/`); commit `[board:feature]`; remove worktree; fire `notify_feature_merged()`
- [ ] 5. `service/review.py` — `rollback_feature(project, feature_id, board_path, aurora_home)`: collect all commits on feature branch since `merge-base` (human and agent); print/return warning listing count + one-line summaries; reset feature branch to `base_branch` HEAD (`git reset --hard $(git merge-base feature base_branch)`); push to origin; reset all tasks in feature to `status: ready`; set feature `status: open`; commit `[board:feature]` + task updates; fire `notify_feature_rolled_back()`
- [ ] 6. `service/review.py` — `reject_feature(project, feature_id, board_path, aurora_home)`: reset feature branch to `base_branch` HEAD; push to origin; set feature `status: closed`; archive board; commit `[board:feature]`; remove worktree; fire `notify_feature_rejected()`
- [ ] 7. `service/review.py` — `archive_board(board_path, project, feature_id)`: move `board/projects/{project}/board/features/active/{feature}/` → `archived/{feature}/`; part of approve and reject flows
- [ ] 8. `service/api/features.py` — wire `POST /api/features/{project}/{feature}/approve`, `/rollback`, `/reject` to review functions; note: `{project}/{feature}` maps to positional `<project/feature>` in CLI; return `409` with conflict details on approve conflict
- [ ] 9. Integration tests — approve: clean merge → `merged`, board archived, worktree gone; approve: conflict → `409`, feature stays `review`; rollback: branch reset to base, all tasks `ready`, feature `open`; reject: branch reset, `closed`, archived; all-tasks-done → feature `review` + Telegram; Telegram dedup: same `(event_type, task_id)` fires once
- [ ] 10. Run `uv run ruff check .` and `uv run mypy service/` — fix all errors

## Change History

- [2026-06-11] Plan deprecated without implementation — stale paths from the
  pre-extraction monolith; review flow to be re-planned under the v2
  architecture.
