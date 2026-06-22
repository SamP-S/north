# 004 — Pipeline Engine

## Summary

Implement the inner pipeline: YAML loader, graph validator, step execution loop with confidence routing, `on_fail` handling, attempt counting, artifact chain passing, and `task_ingest` artifact production. Wire `run_pipeline()` into the supervisor's `run_task()`. Depends on 003a + 003b (cloud and local executors).

## Files to Create / Modify

```
service/
  pipeline/
    __init__.py
    loader.py                  # YAML load + schema validation + graph validation
    executor.py                # run_pipeline(): step loop, routing, artifact chain
  orchestrator/
    task_runner.py             # run_task(): full 6-node per-task flow
definitions/
  pipelines/
    map-code-review.yaml       # example pipeline (from §15.9)
tests/
  test_pipeline_loader.py
  test_pipeline_executor.py
  test_task_runner.py
```

## Todo

- [x] 1. `service/pipeline/loader.py` — `load_pipeline(name, aurora_path, board_path, project)`: check project override first (`board/projects/{project}/pipelines/{name}.yaml`), fall back to global (`definitions/pipelines/{name}.yaml`); parse YAML; validate schema: required top-level fields (`name`, `entry`, `steps`); `name` must match filename stem; each step requires `id`, `agent`, `confidence` (all four levels), `on_fail`; `max_attempts` defaults to `1`
- [x] 2. `service/pipeline/loader.py` — graph validation: `entry` references a valid step id; every `confidence.*` and `on_fail` value is a valid step id or the reserved values `stop`/`done`; all step ids are unique; raise `PipelineLoadError` on any failure (task → `status: blocked`)
- [x] 3. `service/pipeline/executor.py` — `task_ingest(task_state)`: produce artifact `[0]`: frontmatter `agent: system, confidence: high, status: complete, summary: "{task_id} — {title}"`; body = task id + title + task body verbatim; append to `task_state.artifacts`; resolve pipeline name → `PipelineDefinition` (missing or invalid → `status: blocked`, route to `update_board`)
- [x] 4. `service/pipeline/executor.py` — `run_pipeline(pipeline, agent_roster, task_state, worktree_path)`: begin at `pipeline.entry`; for each step: invoke cloud or local executor based on provider inference (§5.6); on `ArtifactParseError`: count attempt, inject parse error as context, retry if attempts < `max_attempts`, else follow `on_fail`; on success: append artifact to chain, route via `artifact.confidence`
- [x] 5. `service/pipeline/executor.py` — routing: `confidence` value → next step id or `stop` or `done`; `stop` → set `final_status = failed`; `done` → set `final_status = done`; `blocked` confidence → set `final_status = blocked`; always return `(final_status, artifacts)`
- [x] 6. `service/orchestrator/task_runner.py` — `run_task(task_state)`: implement the 6-node flow from §5.8: `task_ingest` → `preflight` → `branch_setup` → `agent_prepare` → `run_pipeline` → `update_board`; each node returns a status; failed nodes route directly to `update_board` with appropriate status; `preflight` returning `queued` re-enqueues without board write
- [x] 7. `service/orchestrator/task_runner.py` — `update_board(task_state)`: update task file frontmatter (`status`, `attempts`); write `{id}-{slug}.result.md` with full artifact chain + execution log (§7.2 result format); commit `[system:task]` to board repo; push project feature branch to origin; enqueue Telegram notification
- [x] 8. `definitions/pipelines/map-code-review.yaml` — the example pipeline from §15.9 verbatim
- [x] 9. Unit tests — loader: valid pipeline loads; missing `entry` field rejected; `entry` references non-existent step rejected; `confidence.high` points to non-existent step rejected; step missing `on_fail` rejected; executor: `task_ingest` artifact shape; confidence routing `high` → next step; `blocked` → `final_status=blocked`; `stop` → `final_status=failed`; `done` → `final_status=done`; `ArtifactParseError` retry then `on_fail`; full artifact chain passed to each step
- [x] 10. Run `uv run ruff check .` and `uv run mypy service/` — fix all errors

## Change History

- [2026-06-07] All items complete. 64/64 tests pass, ruff clean, mypy clean. Fixed executor routing order: `blocked` confidence check moved before `stop` check so `blocked` always yields `TaskStatus.BLOCKED` per §5.8. `pipeline_def` stored as dynamic attr on `TaskState.__dict__` pending plan 005 proper field addition.
