# 009 — Borealis Extraction

## Summary

Split the current `aurora/` monolith into two independent services that live as siblings in the **north** monorepo:

- **Borealis** — pure task/board state manager. Owns the `board/` git repo, all board file I/O, task lifecycle (statuses, dependencies, promote/resolve), project/feature/epic state, the git-watcher loop, and Telegram notifications. Has no knowledge of project code repositories. Exposes a FastAPI backend and a `borealis` CLI.
- **Aurora** — agent execution engine. Owns pipeline execution, agent definitions, worktree management, the runner loop, and **feature review** (approve/rollback/reject) — because review involves git operations on project repos. Communicates with Borealis **only** via HTTP REST API. Exposes a FastAPI backend and an `aurora` CLI.

Aurora never reads or writes board files directly. Borealis never imports Aurora Python modules. Borealis never touches project code repositories.

Every mutating Borealis operation (task status, feature status, project register/unregister, requeue, result write) **commits and pushes** to the remote board repo with a structured commit message. The remote is the single source of truth and a full audit log; any board state is recoverable by cloning or checking out a specific commit. `service/board/writer.py` gains a `commit_and_push_board()` helper used by all write paths; standalone `commit_board()` is retained only for the startup reset which runs before the remote is confirmed reachable.

Push failure behaviour: a failed push is logged as an error but does **not** fail the API response — the local commit is intact and the state is consistent. The next successful write will carry the skipped commit to the remote.

The services communicate through the following contract:
- Aurora polls `GET /api/queue` to find eligible tasks.
- Aurora `PATCH`es task status (`in_progress` on claim; `done`/`failed`/`blocked` on completion, with result content).
- After review git operations, Aurora calls Borealis to reflect the outcome in board state (feature status update, task requeue on rollback).
- Borealis writes all board file changes and commits.

---

## Target Directory Structure

```
north/                                  # monorepo root (user handles rename)
  aurora/
    service/
      __init__.py
      config.py                         # aurora-only vars
      main.py                           # /api/health, /api/status, /api/control
      models.py                         # TaskState, Artifact, RunnerState only
      borealis_client.py                # NEW: async httpx client for Borealis API
      execution/                        # unchanged
      git/                              # unchanged (project repo ops)
      orchestrator/
        supervisor.py                   # REFACTORED: polls Borealis, no board state
        task_runner.py                  # REFACTORED: updates via Borealis API
      pipeline/                         # unchanged
      review.py                         # STAYS: git ops on project repos
      api/
        features.py                     # STAYS: approve/rollback/reject endpoints
    cli/
      main.py                           # pared down: runner + review commands
      client.py                         # points to aurora service
      commands/
        control.py                      # unchanged
        lifecycle.py                    # unchanged
        observe.py                      # pared down: runner status + logs only
        review.py                       # STAYS: approve/rollback/reject
    definitions/                        # unchanged (agents, pipelines, tools)
    tests/
    pyproject.toml
    .env
  borealis/
    service/
      __init__.py
      config.py                         # board/notification vars
      main.py                           # all board endpoints + PATCH task status
      models.py                         # board models (TaskModel, FeatureModel, etc.)
      startup.py                        # MOVED from aurora/service/
      api/
        tasks.py                        # NEW: PATCH /api/tasks/{p}/{f}/{t}/status
        features.py                     # NEW: PATCH feature status, POST requeue
      board/                            # MOVED from aurora/service/board/
        loader.py
        parser.py
        writer.py                       # gains commit_and_push_board() helper
      notifications/                    # MOVED from aurora/service/notifications/
        telegram.py
        events.py
      orchestrator/
        git_watcher.py                  # MOVED from aurora/service/orchestrator/
        resolver.py                     # MOVED from aurora/service/orchestrator/
        supervisor.py                   # NEW: board-only loop (sync+promote+resolve)
    cli/
      main.py                           # NEW borealis CLI entry point
      client.py                         # NEW: HTTP client for Borealis API
      commands/
        observe.py                      # MOVED: status/queue/features/logs
        projects.py                     # MOVED from aurora/cli/commands/
    board/                              # MOVED from repo root board/
    tests/
    pyproject.toml                      # NEW
    .env                                # NEW
  docs/                                 # stays at monorepo root
  CLAUDE.md                             # stays at monorepo root
```

---

## New API Contract

### Borealis endpoints Aurora uses

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/queue` | Returns `queued` tasks ordered for execution |
| `PATCH` | `/api/tasks/{project}/{feature}/{task_id}/status` | Update task status + optional result content |
| `GET` | `/api/tasks/{project}/{feature}/{task_id}` | Fetch full task detail (branch, pipeline, body) |
| `PATCH` | `/api/features/{project}/{feature}/status` | Set feature status after review git work |
| `POST` | `/api/features/{project}/{feature}/requeue` | Re-queue tasks after rollback |

### `PATCH /api/tasks/{project}/{feature}/{task_id}/status` body
```json
{
  "status": "in_progress | done | failed | blocked",
  "result_content": "<markdown string or null>"
}
```
Borealis handles: updating task frontmatter, writing the `.result.md` file, committing the board, triggering feature → `review` when all tasks are `done`, and firing Telegram notifications.

### `PATCH /api/features/{project}/{feature}/status` body
```json
{ "status": "merged | closed | review | ..." }
```

### `POST /api/features/{project}/{feature}/requeue` body
```json
{}
```
Resets all `done`/`failed`/`blocked` tasks in the feature back to `queued` and commits the board.

---

## Config Split

### `aurora/.env`
```
BOREALIS_URL=http://127.0.0.1:8001
AURORA_PORT=8000
```

### `aurora/service/config.py` (kept vars)
```
aurora_home, borealis_url, ollama_base_url, ollama_default_num_ctx,
claude_code_max_output_tokens, max_turns_per_call, agent_timeout_s,
runner_concurrency, cooldown_seconds, poll_interval_s, aurora_port
```

### `borealis/.env`
```
BOARD_REPO_SSH_URL=git@github.com:SamP-S/aurora-board-test.git
BOREALIS_PORT=8001
```

### `borealis/service/config.py` (new)
```
board_path (default borealis/board/), board_repo_ssh_url,
poll_interval_s, cooldown_seconds,
telegram_bot_token, telegram_chat_id, borealis_port
```
No `aurora_home` — Borealis never touches project repos.

---

## pyproject.toml Split

### `aurora/pyproject.toml`
- **Keeps:** `fastapi`, `uvicorn`, `pydantic-settings`, `httpx`, `python-frontmatter`, `claude-agent-sdk`, `sse-starlette`
- **Removes:** `gitpython` (no board git ops in aurora)
- **Entry point:** `aurora = "cli.main:_entrypoint"`
- **Packages:** `service`, `cli`

### `borealis/pyproject.toml` (new)
- **Deps:** `fastapi`, `uvicorn`, `pydantic-settings`, `gitpython`, `httpx`, `sse-starlette`, `python-frontmatter`
- **No:** `claude-agent-sdk`
- **Entry point:** `borealis = "cli.main:_entrypoint"`
- **Packages:** `service`, `cli`

---

## Files to Modify / Create / Move

### New files (create)
- `borealis/pyproject.toml`
- `borealis/.env`
- `borealis/service/__init__.py`
- `borealis/service/config.py`
- `borealis/service/main.py`
- `borealis/service/models.py` (board models extracted from aurora)
- `borealis/service/api/__init__.py`
- `borealis/service/api/tasks.py` (new PATCH endpoint)
- `borealis/service/orchestrator/__init__.py`
- `borealis/service/orchestrator/supervisor.py` (board-only loop)
- `borealis/cli/__init__.py`
- `borealis/cli/main.py`
- `borealis/cli/client.py`
- `borealis/cli/commands/__init__.py`
- `aurora/service/borealis_client.py`

### Files moved to borealis (intact or minimal edits to imports)
- `service/board/` → `borealis/service/board/`
- `service/notifications/` → `borealis/service/notifications/`
- `service/startup.py` → `borealis/service/startup.py`
- `service/orchestrator/git_watcher.py` → `borealis/service/orchestrator/git_watcher.py`
- `service/orchestrator/resolver.py` → `borealis/service/orchestrator/resolver.py`
- `cli/commands/projects.py` → `borealis/cli/commands/projects.py`
- `board/` → `borealis/board/`

### Files staying in aurora (unchanged or minimal edits)
- `service/review.py` — stays; does git ops on project repos
- `service/api/features.py` — stays; approve/rollback/reject endpoints; update to call BorealisClient for board state after git work
- `cli/commands/review.py` — stays

### Files modified in aurora
- `service/config.py` — remove telegram vars, add `borealis_url`
- `service/models.py` — remove all board models (keep `TaskState`, `Artifact`, `RunnerState`, `Provider`)
- `service/main.py` — remove queue/projects/task-detail board endpoints; keep `/api/health`, `/api/status`, `/api/control`; keep feature review router
- `service/orchestrator/supervisor.py` — rewrite: poll Borealis API, no board state
- `service/orchestrator/task_runner.py` — rewrite: use `BorealisClient`, no direct board writes
- `service/review.py` — update to call `BorealisClient` for board state changes after git work
- `cli/main.py` — remove board-observation commands (queue/features/projects/register/unregister); keep start/stop/pause/resume/logs/approve/rollback/reject
- `cli/commands/observe.py` — remove queue/features; keep runner status + logs
- `pyproject.toml` — remove `gitpython`
- `.env` — reduce to aurora-only vars

### Files deleted from aurora
- `service/board/` (moved)
- `service/notifications/` (moved)
- `service/startup.py` (moved)
- `service/orchestrator/git_watcher.py` (moved)
- `service/orchestrator/resolver.py` (moved)
- `cli/commands/projects.py` (moved)

---

## Ordered Todo

- [x] 1. Create `borealis/` directory skeleton (`service/`, `service/api/`, `service/board/`, `service/notifications/`, `service/orchestrator/`, `cli/`, `cli/commands/`, `board/`, `tests/`)
- [x] 2. Create `borealis/pyproject.toml` and `borealis/.env`
- [x] 3. Move `board/` → `borealis/board/`
- [x] 4. Move `service/board/` → `borealis/service/board/`; fix imports; add `commit_and_push_board()` to `writer.py`
- [x] 5. Move `service/notifications/` → `borealis/service/notifications/`; fix imports
- [x] 6. Move `service/startup.py` → `borealis/service/startup.py`; fix imports
- [x] 7. Move `service/orchestrator/git_watcher.py` + `resolver.py` → `borealis/service/orchestrator/`; fix imports
- [x] 8. Extract board models (`TaskModel`, `FeatureModel`, `EpicModel`, `ProjectModel`, `BoardState`, `TaskStatus`, `FeatureStatus`, `EpicStatus`) into `borealis/service/models.py`
- [x] 9. Write `borealis/service/config.py`
- [x] 10. Write `borealis/service/orchestrator/supervisor.py` (board loop: sync+promote+resolve; no task execution)
- [x] 11. Write `borealis/service/api/tasks.py` (PATCH task status endpoint)
- [x] 12. Write `borealis/service/api/features.py` (PATCH feature status + POST requeue endpoints)
- [x] 13. Write `borealis/service/main.py` (all board read endpoints + task/feature write endpoints)
- [x] 14. Write `borealis/cli/client.py` (HTTP client targeting Borealis)
- [x] 15. Move `cli/commands/projects.py` → `borealis/cli/commands/`; fix imports
- [x] 16. Write `borealis/cli/commands/observe.py` (status/queue/features/logs)
- [x] 17. Write `borealis/cli/main.py` (borealis CLI: status/queue/features/logs/register/unregister)
- [x] 18. Write `aurora/service/borealis_client.py` (async httpx client: get_queue, claim_task, update_task_status, update_feature_status, requeue_feature)
- [x] 19. Strip `aurora/service/models.py` to execution-only (`TaskState`, `Artifact`, `RunnerState`, `Provider`)
- [x] 20. Rewrite `aurora/service/orchestrator/supervisor.py` (poll Borealis API; no board state)
- [x] 21. Rewrite `aurora/service/orchestrator/task_runner.py` (use `BorealisClient`; no board file writes)
- [x] 22. Update `aurora/service/review.py` to call `BorealisClient` for board state changes post-git
- [x] 23. Update `aurora/service/config.py` (remove telegram vars; add `borealis_url`)
- [x] 24. Update `aurora/service/main.py` (remove board-observation/projects endpoints; keep health/status/control + feature review router)
- [x] 25. Pare down `aurora/cli/main.py` (remove queue/features/projects/register/unregister) and `aurora/cli/commands/observe.py` (keep runner status + logs)
- [x] 26. Update `aurora/pyproject.toml` (remove `gitpython`)
- [x] 27. Update `aurora/.env` to aurora-only vars
- [x] 28. Delete moved files from `aurora/` (`service/board/`, `service/notifications/`, `service/startup.py`, `service/orchestrator/git_watcher.py`, `service/orchestrator/resolver.py`, `cli/commands/projects.py`)
- [x] 29. Run `uv sync` in both `aurora/` and `borealis/`; run `ruff check` on both

---

## Change History

- [2026-06-08] Plan created
- [2026-06-09] Clarified scope: review (approve/rollback/reject) stays in Aurora; Borealis is purely task state. Added `python-frontmatter` to borealis deps. Added feature status + requeue endpoints to Borealis API contract. Removed `aurora_home` from borealis config.
- [2026-06-09] Every Borealis write commits AND pushes to remote board repo. `writer.py` gains `commit_and_push_board()` used by all write paths. Board remote is the authoritative audit log and recovery point.
