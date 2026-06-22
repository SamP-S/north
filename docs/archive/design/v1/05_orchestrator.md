## 5. Orchestrator

### 5.1 Supervisor loop

```
async loop forever:
    if queue_paused:
        await asyncio.sleep(POLL_INTERVAL_S); continue
    await detect_git_changes()                            # sync internal state from board repo HEAD
    if month_spend_estimate >= MONTHLY_SOFT_CAP_USD:
        pause_queue(); notify_once(); continue
    await promote_ready_tasks()                           # §5.4 — ready + cooldown elapsed → queued (board commit)
    candidates = resolve_eligible_tasks()                 # §5.4 (reads in-memory state)
    if candidates:
        task = pick_shallowest(candidates)
        await run_task(task)                              # single worker; blocks supervisor loop until done
    await asyncio.sleep(POLL_INTERVAL_S)
```

**`detect_git_changes()`** runs first each active iteration: compares the board repo `HEAD` against `last_known_head`; if unchanged, returns immediately. On a new commit, diffs changed files since `last_known_head` and processes each:

- **`_feature.md` changes** — validates frontmatter; creates feature branch and worktree for new features; removes worktree for `status: closed`; fires Telegram on invalid frontmatter; updates in-memory feature state
- **Task file changes** — diffs task status against in-memory state; fires `task.transition` SSE events for each change (covers both runner-driven and operator board commits)
- **`projects.yaml` changes** — syncs in-memory project registry; new entries are cloned and initialised; removed entries are unregistered applying the same safety checks as `DELETE /api/projects/{project}`

Updates `last_known_head` on completion.

**`promote_ready_tasks()`** runs a pre-pass each active iteration: scans all `status == ready` tasks across active features; for each where `now ≥ ready_at + COOLDOWN_SECONDS`, transitions the task to `status: queued` and commits a `[system:task]` board update. This makes the cooldown window visible on the board — a task in `queued` has cleared its cooldown and is waiting for the worker.

`resolve_eligible_tasks()` reads in-memory state (already synced by `detect_git_changes()`) and returns tasks where: `status == queued`; every id in `depends_on` is `done`; the parent feature is not paused; every feature in the parent feature's `depends_on` has reached `merged`/`closed`. `month_spend_estimate` is cumulative `total_cost_usd` for the billing cycle, read from `$AURORA_HOME/data/spend.json` on startup and updated after every cloud `query()` call (§5.9).

**While paused**, the loop sleeps for `POLL_INTERVAL_S` on every iteration — no git detection, no promotion, no execution. Cooldown timers do not advance; new features, task transitions, and project registry changes are not picked up until resume. The authoritative pause signal is `RateLimitEvent`; the operator can also pause via `POST /api/control`. On resume, the full loop restarts from the top.

The supervisor loop handles CLI control signals: `pause` stops at the next safe boundary (see [backend API](09_backend-api.md)); the runner idles until `resume` is received.

### 5.2 Per-task execution: node responsibilities

| Node | Reads | Behavior | Writes |
|---|---|---|---|
| `task_ingest` | task file (board repo) | parse frontmatter+body; resolve pipeline from task `pipeline` field (see [pipelines](15_pipelines.md)) — missing or unresolvable pipeline → `status: blocked`; emit artifact `[0]`: task id + title prepended to task body, with system frontmatter (`agent: system, confidence: high, status: complete`) — this seeds the pipeline artifact chain | `pipeline, artifacts[0]` |
| `preflight` | state, registry, env | re-check credit/rate-limit, OAuth valid, deps `done`, feature deps satisfied (§5.4), feature not paused, clone clean + base current, target reachable; reset stale worktree (§5.7) | proceed, or `status→blocked/queued` |
| `branch_setup` | feature, project | idempotent guard: worktree is created eagerly on feature detection (see §8.2 in [git conventions](08_git-conventions.md)); `branch_setup` recreates it if missing (crash recovery) and installs the post-commit hook idempotently (see §8.3 in [git conventions](08_git-conventions.md)) | `worktree_path, branch` |
| `agent_prepare` | aurora `definitions/agents/` + board repo `projects/{project}/agents/` + project repo context | merge agents (see §6.2 in [agent execution](06_agent-execution.md)); assemble declared context files from project worktree; build `AgentDefinition` roster | `agent_roster, context_files` |
| `run_pipeline` | pipeline, agent roster, worktree | execute the pipeline graph (see [pipelines](15_pipelines.md)); each step invokes an agent via `claude-agent-sdk` `query()`, runs checks, routes via confidence; all artifacts accumulated | `artifacts, final_status` |
| `update_board` | full state | update `status` and `attempts` frontmatter in the task file; write all pipeline artifacts in order + execution log to `{id}-{slug}.result.md` (overwritten each run); commit `[system:task]` to board repo; push project feature branch to origin; enqueue notifications | terminal `status` |

### 5.3 `TaskState`

| Group | Fields |
|---|---|
| Identity | `project, epic, feature, task_id, task_path` |
| Target | `pipeline, prompt` |
| Agent roster | `agent_roster, context_files` |
| Git | `branch, worktree_path, commit_hash` |
| Control | `status, error` |
| Telemetry | `started_at, finished_at, input_tokens, output_tokens, cache_read, cache_write, estimated_cost_usd, model_usage` |
| Pipeline | `artifacts, current_step, step_attempts` |

`query()` runs without session resume. `TaskState` is a plain Python dataclass; no graph framework is used.

### 5.4 Queue and dependency resolution

The supervisor loop runs two passes each iteration:

**Pass 1 — `promote_ready_tasks()`:** scans all `status == ready` tasks across active features. For any task where `ready_at` is absent or unparseable, the runner writes the current timestamp to `ready_at` in the task file and commits a `[system:task]` board update — this starts the cooldown window from the moment of detection. Transitions each task where `now ≥ ready_at + COOLDOWN_SECONDS` to `status: queued`, resets `attempts` to `0`, and commits a `[system:task]` board update. The cooldown (`COOLDOWN_SECONDS`, default 300) is the operator's cancel/edit window before a task becomes eligible for execution.

**Pass 2 — `resolve_eligible_tasks()`:** returns tasks where: `status == queued`; every id in the task's `depends_on` is `done` (within the same feature); the parent feature is not paused; **and** every feature in the parent feature's `depends_on` has reached `merged`/`closed`. Ordering: shallowest in the within-feature DAG first, ties by `ready_at`. Task dependencies are within-feature; cross-feature ordering uses **feature** dependencies (is the prerequisite feature merged?).

Dependent tasks are never transitioned to a blocked state — they remain `queued` and are simply skipped by the resolver until their prerequisites are `done`. When the operator fixes a `blocked` or `failed` task and sets it back to `ready`, dependents become eligible automatically on the next resolver loop with no manual intervention. A Telegram notification on the blocker includes the count of dependent tasks that will remain waiting.

### 5.5 Concurrency

`RUNNER_CONCURRENCY=1` — one task at a time (matches the 6 GB single-model GPU). Multiple features may be eligible; the single worker serializes them. Revisit once local throughput is characterized.

### 5.6 Model routing and resolution

Each agent definition declares its own model. Provider inference uses an explicit allow-list: model names matching `claude-*`, `opus`, `sonnet`, or `haiku` ⇒ `anthropic` (invoked via Agent SDK `query()`); everything else ⇒ `local` (invoked via Ollama `local_executor` at `OLLAMA_BASE_URL/api/chat`, see §6.5 in [agent execution](06_agent-execution.md)). The allow-list approach prevents local model alias names from accidentally matching cloud prefixes. Project agent overrides take precedence over globals (see §6.2 in [agent execution](06_agent-execution.md)). Escalation is expressed as purpose-built agents in the pipeline graph (see [pipelines](15_pipelines.md)), not a global escalation model.

### 5.7 Crash / restart behavior

On restart, any `in_progress` task → `queued`; no partial resume. This includes tasks that were paused mid-pipeline — pause state (current step + accumulated artifacts) is in-memory only and is discarded on service stop; the task re-runs from scratch on next pickup. During `preflight`, a worktree at `$AURORA_HOME/worktrees/{project}/{feature}` whose feature has a stuck task is hard-reset (`git reset --hard && git clean -fd`). The runner writes a pid/lock.

### 5.8 Control flow

`run_task()` is a plain `async def` function. The sequence:

```
task_ingest ──► preflight ──► branch_setup ──► agent_prepare ──► run_pipeline ──► update_board
     │                │                                │
     │ (blocked)      │ (queued)                       │ (blocked)
     ▼                ▼                                ▼
update_board       re-enqueue                    update_board
                  (no board write)
```

`run_pipeline` executes the pipeline graph defined in `definitions/pipelines/` — see [pipelines](15_pipelines.md) for the full pipeline specification. `run_pipeline` always returns a terminal status (`done`, `failed`, `blocked`) regardless of which path through the pipeline graph was taken — `stop` routing sets `failed`, `blocked` confidence sets `blocked`. `update_board` always runs after `run_pipeline`.

Every terminal path routes through `update_board` — there are no silent exits. The only exception is `preflight` returning `status→queued` (dependency not yet met, cooldown not passed), which re-enqueues without a board write as the status has not changed.

**`task_ingest` failure** (missing or unresolvable `pipeline` field) sets `status→blocked` and routes directly to `update_board`, skipping `preflight`, `branch_setup`, `agent_prepare`, and `run_pipeline`.

`preflight` returning `status→blocked` (feature paused, worktree unrecoverable, OAuth invalid) and infrastructure failures in `branch_setup` or `agent_prepare` route to `update_board` with `status→blocked`. Both `blocked` and `failed` fire a Telegram notification including the count of dependent tasks that will remain queued until resolved.

**`agent_prepare` failure conditions:** any agent name referenced in the pipeline steps cannot be resolved to a definition (not found in globals or project overrides), or a definition file contains invalid YAML frontmatter. Missing context paths declared in an agent's `context` field are logged as a warning and skipped — they do not cause `agent_prepare` to fail.

### 5.9 Spend tracking

Monthly cloud spend is persisted to `$AURORA_HOME/data/spend.json` — a small file written to disk (not committed to any repo) after every cloud `query()` call:

```json
{"cycle_start": "2026-06-01", "total_usd": 4.23}
```

On startup the runner reads this file; if the current day-of-month matches `BILLING_CYCLE_DAY` and `cycle_start` is in a prior month, it resets the counter to zero with the current date. After each `query()` call, `total_cost_usd` from the `ResultMessage` is added and the file is written atomically (write to `$AURORA_HOME/data/.spend.json.tmp`, rename). Local `query()` calls (provider `local`) contribute `0.0`. `$AURORA_HOME/data/` is created by the install script if it does not exist.
