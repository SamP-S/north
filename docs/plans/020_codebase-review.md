# North — Codebase Review: Prioritized Improvements

## Context

Requested review of the whole monorepo (Aurora + Borealis, ~4.4k lines) to produce an
ordered list of suggested fixes, refactors, and features. Current health: 149 tests pass,
ruff clean. The list below is ordered by priority — correctness bugs first, then
robustness, then architecture, then features.

## P1 — Correctness bugs (break the core task flow)

1. **`TaskRunner` reads stale, pre-extraction fields from the queue payload** — every
   task gets BLOCKED.
   - `TaskRunner.run_task()` (`aurora/aurora/service/orchestrator/task_runner.py`) reads
     `task_dict["branch"]` (blocks when empty) and `task_path` (falls back to
     `/tmp/<id>.md`), but `_queue_entry()` in `borealis/borealis/service/main.py:96`
     intentionally returns only board-level fields
     (`task_id/title/status/project/feature/ready_at/pipeline/body`). Aurora is the
     out-of-date side: `branch` is feature-level data already exposed by
     `GET /api/projects/{p}/features/{f}` — `review.py` fetches it correctly via
     `BorealisClient.get_feature()`; `task_path` is Borealis-local and Aurora should not
     touch it at all.
   - Fix in Aurora: have `TaskRunner` call `client.get_feature(project, feature)` to get
     `branch`, and remove the `task_path` dependency from `_TaskInfo`/`TaskState` (or
     make it Aurora-local). Add an end-to-end contract test (Borealis queue payload →
     TaskRunner) so the two services can't drift again.

2. **`rstrip(".git")` strips characters, not a suffix** — `register_project()` in
   `borealis/borealis/service/main.py:166`. `"tagit.git".rstrip(".git")` → `"ta"`.
   Use `removesuffix(".git")` after `rstrip("/")`.

3. **Borealis supervisor loop dies permanently on any exception.**
   `borealis/borealis/service/orchestrator/supervisor.py` — `except Exception` wraps the
   whole `while True`, so one transient git error stops promotion/reload forever (Aurora's
   supervisor does this correctly per-iteration). Move the handler inside the loop.

## P2 — Robustness / operational gaps

4. **Git watcher never fetches the remote.** `git_watcher.detect_git_changes()` only
   compares the local HEAD, so board edits pushed from elsewhere are invisible until a
   local commit happens. Add a periodic `fetch` + `pull --rebase` (reusing the conflict
   handling already in `board/writer.py:push_board`).

5. **Crash-recovery / stale `in_progress` tasks.** Aurora marks a task `in_progress`
   before running it; if Aurora dies mid-run, the task sits first in the queue and is
   silently re-run with stale state, and a partially-created worktree is reused
   (`if not worktree_path.exists()` skips re-creation without validating the branch).
   Define explicit resume semantics: stamp a lease/heartbeat, or reset `in_progress` →
   `queued` on Aurora startup.

6. **Shared mutable board state with no locking.** Borealis endpoints are sync (run in a
   threadpool) and mutate `board_state` dicts + the git index while the async supervisor
   reloads/replaces the state. Low traffic makes it rare, not safe. Introduce a single
   `asyncio.Lock` (make endpoints `async`) around mutations + reloads, or funnel all
   writes through the supervisor.

7. **Aurora `/api/status` is a stub** — `active_task`, `active_project`, `oauth_health`
   are hardcoded (`aurora/aurora/service/main.py`). Either wire them to real state
   (Supervisor knows the running task) or drop the fields; lying status endpoints are
   worse than missing ones.

8. **Deprecated FastAPI `@app.on_event("startup")`** in both services (the pytest warning
   is this). Migrate to lifespan context managers.

## P3 — Architecture / refactors

9. **Board lives inside the source tree.** `_BOARD_PATH = Path(__file__)/../../board` in
   `borealis/service/main.py`. It should be under `NORTH_HOME` (settings-driven, like
   Aurora's homes) — keeps installs relocatable and the repo clean.

10. **TaskRunner silently drops board/project-level overrides.** It passes `aurora_path`
    as `board_path` for both pipeline loading and agent definitions ("falls back to
    global pipelines only"). Either remove the override layers from
    `pipeline/loader.py` / `agent_prepare.py` (dead capability) or have Aurora fetch
    board-level config from Borealis. Pick one; the half-state is confusing.

11. **Consolidate the legacy runtime onto the adapter seam.** `runtime/legacy.py` wraps
    `execution/cloud.py` + `execution/local.py`, which still carry their own
    status/result vocabulary (`CloudStepResult` with `rate_limited`/`auth_failed` flags
    duplicating `Outcome`). Fold cloud/local into proper `AgentRuntime` implementations
    and delete the translation layer. Also remove fragile bits while there:
    `type(msg).__name__ == "RateLimitEvent"` string matching and the global
    `os.environ["CLAUDE_CODE_MAX_OUTPUT_TOKENS"]` mutation in `cloud.py`.

12. **`BorealisClient` creates a new `httpx.AsyncClient` per call and has no
    retry/backoff.** Hold one client for the service lifetime; add bounded retries for
    the status-report path (a failed `_report` currently loses the task result entirely —
    it's only logged).

13. **Duplicate prompt-building code.** `_build_system_prompt`/`_build_user_prompt` are
    copy-pasted in `runtime/opencode.py` and `execution/cloud.py`. Move to one module
    (falls out of item 11 naturally).

14. **`runner_concurrency` setting exists but is unused** — supervisor runs strictly
    one task. Either implement bounded concurrency (per-project worktree exclusivity
    needed) or delete the setting until it's real.

## P4 — Features / quality-of-life

15. **mypy in CI/dev loop.** `pyproject.toml` configures `mypy --strict` but nothing runs
    it (no CI at all). Add a GitHub Actions workflow: `uv sync`, `ruff check`, `mypy`,
    `pytest`. Cheapest high-value item on this list.

16. **Real task log/result history.** `TaskRunner._report` fabricates a log
    (`status: in_progress` stamped at finish time) and rebuilds the whole result blob.
    Stream per-step events (step started/finished, attempts, outcome) into the result
    file or a structured log so failed runs are debuggable.

17. **Observability.** Both services log to journald only. Add per-task structured
    logging (task_id in every record via `logging` extra/contextvars) and a
    `/api/tasks/{id}/events` endpoint; this is the thing you'll miss first when a
    pipeline misbehaves.

18. **Feature lifecycle completion.** `update_task_status` auto-promotes features to
    `review` when all tasks are done, but merge/close still appears manual — finish the
    review→merge flow in `aurora/service/review.py` (auto-PR or auto-merge per project
    setting).

## Verification

- Item 1: new integration test driving Borealis `/api/queue` output directly into
  `TaskRunner.run_task` against a temp board repo (pattern exists in
  `aurora/tests/integration/test_full_task_run.py`).
- Items 2–8: unit tests alongside the existing suites (`borealis/tests/test_api.py`,
  `test_startup.py`); `uv run pytest` must stay green.
- Item 15: CI workflow proves itself on first push.

## Change history
- [2026-06-11] Initial review written.
- [2026-06-11] P1 items 1–3 implemented:
  1. `TaskRunner` now fetches `branch` via `BorealisClient.get_feature()` when the queue
     payload lacks it; `task_path` removed from `TaskState`/`_TaskInfo` (dead field).
     Integration tests rewritten to use the real `/api/queue` payload shape; new
     contract test `test_run_task_feature_without_branch_blocks`.
  2. `rstrip(".git")` → `removesuffix(".git")`, extracted as `derive_project_name()`
     with a regression test.
  3. Borealis supervisor exception handling moved inside the loop so transient errors
     no longer kill it.
  151 tests pass, ruff clean.
- [2026-06-11] Plan file renamed to `020_codebase-review.md` (019 was taken).
- [2026-06-11] P2 items 4–8 implemented:
  4. `sync_remote()` added to `git_watcher.py` (pull --rebase, abort on conflict,
     no-op without an `origin` remote); called every supervisor tick. New tests in
     `borealis/tests/test_git_watcher.py`.
  5. Aurora `Supervisor.recover_stale_tasks()` resets `in_progress` → `queued` via the
     Borealis API on startup (covers Aurora crashes; Borealis restarts were already
     covered by its own startup reset). Supervisor now only picks `status == "queued"`
     entries. New tests in `aurora/tests/test_supervisor_recovery.py`.
  6. `board_lock` (threading.Lock) added in `api/deps.py`; `get_board_context` is now a
     yield dependency holding the lock for the request; Borealis supervisor tick runs
     under the lock in a worker thread (`asyncio.to_thread`); register/unregister
     endpoints wrapped. Read-only listing endpoints remain lock-free (atomic state-ref
     swap on reload keeps them consistent).
  7. Aurora `/api/status` now reports real `active_task`/`active_project` from the
     supervisor; `oauth_health` left as "unknown" (CLI still prints it).
  8. Both services migrated from `@app.on_event("startup")` to lifespan context
     managers; supervisor tasks cancelled on shutdown.
  156 tests pass, ruff clean, FastAPI deprecation warnings gone.
- [2026-06-11] P3 items 9–14 implemented:
  9. Board moved out of the source tree: `settings.board_path` (default
     `~/.north/borealis/board`, env-overridable as `BOARD_PATH`) now used by
     `main.py`; `install.sh` and README updated. Existing installs need a one-time
     move or `BOARD_PATH` override pointing at the old location.
  10. Board/project override layers removed (Aurora has no board checkout):
      `load_pipeline(name, aurora_path)`, `load_agent_definitions(aurora_path)`,
      `task_ingest(task, state, aurora_path)`. Three override tests deleted.
  11. Runtimes consolidated on the `StepResult`/`Outcome` seam: `run_cloud_step` and
      `run_local_step` now return `StepResult` directly; `CloudStepResult`/
      `LocalStepResult` deleted; `LegacyRuntime` is a thin dispatcher. Cloud step now
      uses real `RateLimitEvent`/`ResultMessage` isinstance checks, passes
      `model=agent_def.model` to the SDK (previously never passed!), and sets
      `CLAUDE_CODE_MAX_OUTPUT_TOKENS` via `ClaudeAgentOptions(env=...)` instead of
      mutating `os.environ`. Local step honours `settings.agent_timeout_s` instead of
      a hardcoded constant; Ollama-unreachable now maps to `Outcome.ERROR`.
  12. `BorealisClient` holds one persistent `httpx.AsyncClient` (`aclose()` added);
      `update_task_status` retries 3× with backoff. `TaskRunner` takes an optional
      shared client; the supervisor passes its own.
  13. Prompt building deduplicated into `aurora/service/runtime/prompts.py`
      (used by cloud, local, and opencode runtimes).
  14. Unused settings removed: `runner_concurrency`, `cooldown_seconds`,
      `max_turns_per_call`, `claude_code_max_output_tokens` (the latter superseded by
      per-agent `max_output_tokens`).
  153 tests pass (3 override tests removed), ruff clean.
- [2026-06-11] P4 item 15: `.github/workflows/ci.yml` added — ruff + pytest gate; mypy
  runs as a separate informational job (47 pre-existing strict-mode errors to pay down
  before making it required).
- [2026-06-11] P4 items 16–18 implemented:
  16. Real task event log: `TaskState.events` + `log()` (`TaskEvent` dataclass);
      pipeline executor logs step start/attempt/outcome, TaskRunner logs lifecycle;
      `_report` renders actual timestamped events under `## Log` instead of the
      fabricated two-line log.
  17. Observability: `aurora/service/logctx.py` — `task_id` contextvar + logging
      filter/format wired in the Aurora lifespan; TaskRunner sets/resets the var per
      task so every journald record carries `[task:<id>]`. New `GET /api/events` on
      Aurora returns the live event log of the running task (supervisor tracks the
      active runner).
  18. Review→merge automation: `auto_merge` flag on projects (models, parser, writer,
      register API + `--auto-merge` CLI flag, surfaced in `/api/review` entries).
      Aurora's supervisor polls the review list when idle and calls
      `approve_feature()` for flagged features; conflicts (409) are logged and not
      retried until the feature leaves and re-enters review.
  New tests: test_logctx, test_auto_merge, parser auto_merge round-trip, event-log
  assertions in test_full_task_run. 159 tests pass, ruff clean.
- [2026-06-12] M1 (plan 022) retired several open items wholesale: item 10
  (override layers — `pipeline/loader.py` + `agent_prepare.py` deleted), item 11
  (legacy runtime + cloud/local executors deleted entirely, including the
  `RateLimitEvent` string match and env mutation), item 13 (duplicate prompt
  builders deleted with `runtime/prompts.py`). Item 16 superseded by the richer
  TaskRunner v2 result (handoff notes + gate reports + session manifest).
  Still open from this review: item 12 (`BorealisClient` per-call client, no
  retry on `_report`), item 14 (`runner_concurrency` — setting no longer
  exists; bounded concurrency itself remains future work), mypy debt (item 15
  informational job).
- [2026-06-12] M2/M3 (plans 023/024) follow-up: item 12 is now **closed** —
  `BorealisClient` holds one `AsyncClient` for its lifetime and both
  `update_task_status` and the new `update_conversation_status` retry 3×
  with backoff. Remaining open from this review: item 14 (bounded
  concurrency) and the mypy informational job (item 15) only.
