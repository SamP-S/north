# 002 — Queue & Dependency Resolver

## Summary

Implement the supervisor loop with a stub executor, `promote_ready_tasks()`, `resolve_eligible_tasks()`, `detect_git_changes()`, and the spend tracker. Goal: the loop runs, detects board changes, promotes tasks through cooldown, resolves dependency ordering, and picks the next eligible task — without actually executing anything.

## Files to Create / Modify

```
service/
  orchestrator/
    __init__.py
    supervisor.py              # main async supervisor loop
    resolver.py                # promote_ready_tasks + resolve_eligible_tasks
    git_watcher.py             # detect_git_changes()
    spend.py                   # spend.json read/write + billing-cycle reset
  main.py                      # wire supervisor loop as asyncio.create_task() on startup
tests/
  test_resolver.py
  test_spend.py
```

## Todo

- [x] 1. `service/orchestrator/spend.py` — read/write `$AURORA_HOME/data/spend.json`; `add_cost(usd)` writes atomically (tmp → rename); `reset_if_new_cycle()` checks `BILLING_CYCLE_DAY` and resets counter when month rolls over; called on startup and after every cloud `query()`
- [x] 2. `service/orchestrator/resolver.py` — `promote_ready_tasks(board_state, now)`: scan `status==ready` tasks; write `ready_at` if absent and commit `[system:task]`; transition to `queued` + reset `attempts` when cooldown elapsed; commit `[system:task]`
- [x] 3. `service/orchestrator/resolver.py` — `resolve_eligible_tasks(board_state)`: return tasks where `status==queued`, all `depends_on` ids are `done`, parent feature not paused, all feature `depends_on` are `merged`/`closed`; order shallowest DAG depth first, ties by `ready_at`
- [x] 4. `service/orchestrator/git_watcher.py` — `detect_git_changes(board_repo, last_head)`: compare HEAD to `last_known_head`; on new commit, diff changed files; handle `_feature.md` changes (validate frontmatter, fire SSE `task.transition`), task file changes (diff status vs in-memory, fire SSE events), `projects.yaml` changes (sync registry); return new `last_head`
- [x] 5. `service/orchestrator/supervisor.py` — async supervisor loop matching §5.1 exactly: paused check → `detect_git_changes()` → soft-cap check → `promote_ready_tasks()` → `resolve_eligible_tasks()` → `pick_shallowest()` → stub `run_task()` → `asyncio.sleep(POLL_INTERVAL_S)`; hold `queue_paused` flag; handle `pause`/`resume` control signals
- [x] 6. `service/main.py` — launch supervisor as `asyncio.create_task()` on FastAPI startup; wire `POST /api/control` to set `queue_paused` flag; wire `GET /api/queue` to read live in-memory state
- [x] 7. Unit tests — cooldown boundary (not yet / exactly elapsed / past elapsed); `depends_on` blocking (prerequisite not done); feature-level `depends_on` blocking; shallowest-first ordering; spend reset on billing day; spend no-reset mid-cycle
- [x] 8. Run `uv run ruff check .` and `uv run mypy service/` — fix all errors

## Change History

- [2026-06-07] All items complete. 20/20 tests pass, ruff clean, mypy clean (13 source files). Fixed task/feature dep lookup to use full `{project}/{feature}/{task_id}` keys matching `BoardState` dict layout. `_supervisor` wired into FastAPI startup; SSE queue drains `task.transition` events from `git_watcher`.
