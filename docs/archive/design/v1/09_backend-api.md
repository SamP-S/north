## 9. Service API (FastAPI)

### 9.1 Conventions

Base path `/api`; JSON in/out. Auth: none at the app layer; uvicorn binds `127.0.0.1`. This API is the IPC surface for the CLI — read-only observation and runner control. Board reads and writes happen directly on the board repo filesystem; this API does not expose board CRUD.

### 9.2 Endpoints

**Runner observation**

| Method | Path | Notes |
|---|---|---|
| GET | `/api/status` | Service health, current task, runner state (`running\|paused\|idle`), OAuth health, credit estimate |
| GET | `/api/queue` | Pending and active tasks; optional `?project=` filter; ordered by dependency depth then `ready_at` |
| GET | `/api/projects` | List all registered projects |
| GET | `/api/features` | All active features across all projects; optional `?project=` filter |
| GET | `/api/review` | Features in `status: review` across all projects; optional `?project=` filter |
| GET | `/api/tasks/{project}/{feature}/{task_id}` | Full task detail — parsed frontmatter, body, and result file contents if present |

**Live stream**

| Method | Path | Notes |
|---|---|---|
| GET | `/api/events` | SSE stream; emits runner state changes and live agent session output |

**Control**

| Method | Path | Notes |
|---|---|---|
| POST | `/api/control` | Body: `{"action": "pause" \| "resume"}` |

**Project management**

| Method | Path | Notes |
|---|---|---|
| POST | `/api/projects/register` | Body: `{"ssh_url": "..."}` — clones repo, creates board dirs, updates `projects.yaml`, commits `[board:project]`; returns `409 Conflict` if a project with the same name is already registered (name is the key; same URL under a different name is allowed) |
| DELETE | `/api/projects/{project}` | Unregisters project; always proceeds regardless of task state; removes board, clone, worktrees, `projects.yaml` entry, commits `[board:project]`; response includes a warning listing any active or in-progress features so the operator knows what work is being discarded |

**Feature review**

| Method | Path | Notes |
|---|---|---|
| POST | `/api/features/{project}/{feature}/approve` | Merges feature branch into `base_branch`; on conflict returns `409` with conflict details; on success archives board, removes worktree |
| POST | `/api/features/{project}/{feature}/rollback` | Resets feature branch to `base_branch` HEAD; resets all tasks to `ready`; feature → `open` |
| POST | `/api/features/{project}/{feature}/reject` | Resets feature branch to `base_branch` HEAD; feature → `closed`; archives board, removes worktree |

Note: `approve`, `rollback`, and `reject` are addressed via `{project}/{feature}` in the URL — matching the `<project/feature>` positional format used by the CLI.

**System**

| Method | Path | Notes |
|---|---|---|
| GET | `/api/health` | |

### 9.3 Control semantics

Two levels of control, handled differently by the CLI:

**Process lifecycle** — managed via systemd, not the API:

| CLI command | Behaviour |
|---|---|
| `aurora start` | `systemctl start aurora` — starts the service |
| `aurora stop` | `systemctl stop aurora` — stops the service entirely |

**Runtime control** — API calls to the running service:

| Action | Agent running | Agent idle |
|---|---|---|
| `pause` | Stops the current agent session; resets the worktree to the pre-step commit hash (`git reset --hard <pre-step-hash>`, discarding both uncommitted work and any commits made during the interrupted step); records the current pipeline step in memory; board status remains `in_progress`; runner stops picking up new tasks | Runner stops picking up new tasks; nothing to discard |
| `resume` | — | Restarts the agent from the recorded pipeline step with all prior artifacts as context; runner resumes normal loop |

Pause is not immediate — the runner stops at the next safe boundary:
- If in `task_ingest`, `preflight`, `branch_setup`, or `agent_prepare`: completes the current node then halts before entering `run_pipeline`. Nothing to discard.
- If in `run_pipeline`: stops the active agent session and resets the worktree to the pre-step commit hash.
- If in `update_board`: allowed to complete — interrupting would leave the board inconsistent. Runner then halts.

On resume after an active-agent pause, the interrupted step is restarted with a fresh `query()` call — the agent has no knowledge a pause occurred. The runner holds the following in memory for the duration of the pause:

- The **current step id** — so resume re-enters the correct step
- **The pre-step commit hash** — used to reset the worktree on pause; the step restarts against a clean worktree identical to the state before the step first ran
- **All artifacts from completed prior steps** — passed as context to the fresh agent instance, identical to a normal step transition
- The **attempt count for the interrupted step** — unchanged, so the step does not receive a free retry

On pause, the worktree is reset to the pre-step commit hash (`git reset --hard <pre-step-hash>`), discarding both uncommitted changes and any commits the agent made during the interrupted step. The agent restarts the step against a clean worktree.

**Pause state is in-memory only.** If the service is stopped while paused (`aurora stop`), the recorded step and artifacts are discarded. On restart, the task resets to `queued` (see §5.7 in [orchestrator](05_orchestrator.md)) and re-runs from the beginning of the pipeline on next pickup.

**Edge cases:**

| Scenario | Behaviour |
|---|---|
| `pause` when already paused | No-op; API returns `{"message": "runner is already paused"}` |
| `resume` when not paused | No-op; API returns `{"message": "runner is not paused"}` |
| `pause` when idle (no active task) | Runner stops picking up new tasks; no step or artifacts to record |
| `resume` after an idle pause | Runner resumes normal task pickup; no step to re-enter |

### 9.4 SSE event types

| Type | Payload |
|---|---|
| `runner.state` | `{state: running\|paused\|idle, task_id?, project?}` — `idle` means running but no active task |
| `agent.output` | `{task_id, project, chunk}` — streamed lines from the active agent session |
| `task.transition` | `{task_id, project, feature, from_status, to_status}` |

`task.transition` events are emitted by the supervisor loop (see §5.1 in [orchestrator](05_orchestrator.md)) when it detects that the board repo `HEAD` has advanced. The runner re-reads changed task files, diffs status against its last known in-memory state, and publishes a `task.transition` event for each status change. This covers both runner-driven transitions and operator board commits.
