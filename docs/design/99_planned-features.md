## 99. Planned Features

### 99.1 Overview

The roadmap of this project entails a number of planned features that were deemed too complex to be attempted within the early MVP design. Features marked **[HIGH PRIORITY]** should be considered before others; remaining items have no fixed order.

### 99.2 Roadmap

#### LangGraph Integration **[HIGH PRIORITY]**
Replace Aurora's custom `run_pipeline()` step-execution loop with LangGraph as the inner pipeline engine. Delivers durability, human-in-the-loop, and visual debugging without changing Aurora's pipeline YAML format or agent definitions.

- **Phase 1 — inner pipeline (priority):** `PipelineCompiler` compiles `definitions/pipelines/*.yaml` into a LangGraph `StateGraph` at startup; `run_pipeline()` calls `graph.ainvoke()`. YAML format unchanged; all pipelines cached by name at service start.
- **Phase 2 — outer orchestrator (deferred):** wrap the 6-node per-task flow (`task_ingest → … → update_board`) as LangGraph nodes once `RUNNER_CONCURRENCY > 1` or node-level crash recovery is needed. `TaskState` is already compatible with LangGraph's typed state pattern; no internal logic changes required.
- **Concept mapping:** artifact chain → `Annotated[list[Artifact], operator.add]` state field (append-only); confidence routing → `add_conditional_edges()` reading `state["artifacts"][-1].confidence`; retry loop → cyclic back-edge when attempts < `max_attempts`; `done`/`stop` → terminal nodes setting `final_status` before `END`.
- **Node-level checkpointing:** state persisted to SQLite/Postgres after every node; pipeline resumes from last completed step on crash/restart, replacing the current full-rerun behaviour (§5.7). Switchable backend — `SqliteSaver` for v1.
- **Human-in-the-loop at step level:** `interrupt()` pauses inside `run_pipeline` at any node boundary, persists full state, and waits for operator input — more granular than the current pipeline-level pause (§9.3).
- **LangGraph Studio (Self-Hosted Lite, free):** visual graph topology, execution path highlighting, time-travel debugging (rewind to any checkpoint, fork from that state). Inspector/debugger only — pipelines remain YAML files in Git.
- **Why not Langflow / Sim.ai:** both are separate services (loose coupling); intermediate artifacts not exposed via REST API; pipeline definitions leave Git; Aurora's safety layer (`GLOBAL_BASH_DENYLIST`, tool scoping) would need reimplementing inside their component systems.
- **Working examples:** `docs/examples/01_pipeline_python.py` (native Python) and `docs/examples/02_pipeline_yaml.py` (YAML-compiled).

---

#### Reconciliation
Tracks operator commits made directly to project repos (from any machine) and keeps the board in sync.
- **Trigger paths:** local post-commit hook catches on-server commits; periodic `git fetch` detects new commits pushed from another machine
- **Reconciliation agent:** reads the diff and commit message; marks a task `done` if the diff clearly completes it; appends to a `manual-changes.md` log if the work has no task; triggers a conflict flow if the changed files overlap with an `in_progress` agent task
- **Conflict flow:** affected task → `conflicted` (new planned task status, distinct from feature statuses); feature queue pauses; conflict record written to `board/features/active/{feature}/conflicts/{timestamp}.md`; Telegram fires
- **Resolution options (operator-decided):**
  - *Keep mine* — agent work discarded, worktree reset, task → `done`
  - *Let agent continue* — operator's changes rebased into the feature branch, task → `queued`
  - *Resolve manually* — task → `manual-resolution` (new planned task status); feature holds until operator commits resolution
- Triage runs local-first, escalating only on large/ambiguous diffs

#### Context Agents
Automated agents for maintaining and evolving project context in `docs/ctx/`.
- **`context_keeper`:** runs after feature merges and on a weekly drift sweep; operates on main with a serialized writer lock; reads diffs and conservatively updates `docs/ctx/` files (no wholesale rewrites, ≤1–2 lines per concept, warns if a file exceeds 150 lines); commits `[agent:context_keeper]` and appends a summary to `changelog.md`; Telegram fires on update
- **`architect`:** planning, spec-writing, and task decomposition; reserved for Opus-class models; outputs are proposed tasks written to `board/.../tasks/proposed/` — never executed directly; operator promotes to staging or deletes
- **`onboarder`:** bootstraps `docs/ctx/` for a newly registered project
- **Context update requests:** machine-written change requests staged for operator review before landing

#### Memory
- Long-term memory store per project that agents can write to and request from
- Ensures relevant context is not lost across pipeline sessions (each session is stateless)
- Likely a structured Markdown or lightweight store committed to aurora

#### Proposed / Staging Workflow
Two-stage operator review for agent-generated task proposals.
- `proposed/` — architect or planner agent writes candidate tasks here; never auto-executed
- `staging/` — operator promotes a proposal; becomes a real `draft` task when moved to `tasks/` with valid frontmatter
- Supports planner agents that suggest new work based on current board/context state

#### Metric Collection
Full telemetry on agent execution stored in a SQLite database.
- **`task_runs` table:** one row per pipeline step invocation; rows share `task_id`
- **Fields:** project/feature/task ids; pipeline name, step id, agent name, model, provider; status, confidence; step attempts; check results; input/output/cache token counts; estimated cost; commit hash
- **Cost model:** cloud draws on the Pro $20/month Agent SDK credit via OAuth; `estimated_cost_usd` is a client-side estimate at standard API rates; local runs cost `0.0`
- **Daily backup:** integrity check → SQL dump → committed to the aurora repo (path TBD when storage strategy is decided)

#### Frontend UI
A web-based board interface served by the aurora service.
- Project/feature/task timeline tree/graph
- Task kanban boards per feature branch with drag-and-drop status transitions
- Context view pages with git history
- Agent view + edit pages with diff viewer and git history
- Metric analytic pages
- Conflict resolution UI

#### Advanced Testing
- Full demo project smoke suite as end-to-end proof of concept
- Exercises: two features, one deliberate conflict (including an off-server pushed commit), one dependency failure, escalation, merge with archive in aurora, board restore from aurora remote

#### Parallel / Concurrency
- Allow multiple tasks/pipelines to execute in parallel where feature dependencies permit
- Currently blocked by single-model GPU (6 GB VRAM); one 7B model resident at a time
- Revisit once local throughput is characterised (`RUNNER_CONCURRENCY=1` for now)

#### Artifact Context Window Management
In long pipelines with retries, the accumulated artifact chain passed to each step can exceed local model context windows (`num_ctx`). Ollama truncates silently when this happens. Two candidate mitigations to evaluate once real pipeline authoring experience exists:
- **`max_context_artifacts`** — per-step config limiting how many prior artifacts are injected (most recent N only)
- **`MAX_ARTIFACT_CHARS`** — global or per-agent cap truncating each artifact body before injection

The right approach depends on which artifacts agents actually need (last step only vs full history) and how verbose real pipeline outputs turn out to be in practice.

#### Pipeline Pass/Fail vs Confidence Routing
Investigate whether a first-class pass/fail signal (distinct from agent confidence) is needed at the engine level — for example, a deterministic post-step validator that routes independently of `confidence`. Current design delegates all quality gates to agent steps using confidence routing. To be revisited once pipeline authoring experience is established and patterns emerge.

#### SSE Event Stream

Removed in the API cleanup pass — `GET /api/events`, `git_watcher.sse_event_queue`,
the CLI `borealis logs` command, and `BorealisClient.sse_stream` were all dead code
(no publisher ever populated the queue). Re-add once the frontend UI needs live
updates.

- **Board reload** — push `{"type": "board_reloaded"}` from `detect_git_changes`
  when a new commit triggers a full reload, so the frontend knows to re-fetch
  project/feature/task lists
- **Task status changes** — push an event from `update_task_status` (including the
  done → feature-review cascade) so kanban-style views update live
- **Feature status changes** — push an event from `update_feature_status` /
  `requeue_feature` so the review list updates live
- **Queue/orchestrator activity** — push an event when the supervisor promotes a
  task to `queued` or it starts running, for a live "current activity" panel
- Keep payloads lightweight (type + project/feature/task ids) — the frontend
  re-fetches the relevant resource rather than receiving full state over SSE

#### Telegram Notifications

The notification infrastructure was removed in v1 to reduce scope. Both services will need it restored when operational alerting is wanted.

**Borealis** (`borealis/borealis/service/notifications/`)

Re-create two files matching the deleted implementation:

- `telegram.py` — `send_telegram(message)`: POST to Telegram bot API; 3 attempts, 5 s fixed delay; no-op if token/chat not configured
- `events.py` — `EventDeduper` class (dedup by `(event_type, task_id)` per task run); typed `notify_*` helpers:
  - `notify_invalid_feature_frontmatter(project, feature_id, error)` — call from `orchestrator/git_watcher.py` `_update_feature` on `ParseError` and on directory-name mismatch
  - `notify_task_done(project, feature_id, task_id)` — call from `api/tasks.py` when a task transitions to `done`
  - `notify_task_terminal_failure(project, feature_id, task_id, reason, dependent_count)` — call from `api/tasks.py` on `failed`/`blocked`
  - `notify_feature_review(project, feature_id)` — call from `api/tasks.py` when all tasks in a feature reach `done`
  - `notify_feature_merged(project, feature_id)` — call from `api/features.py` `PATCH` to `merged`
  - `notify_feature_rolled_back(project, feature_id, commit_count)` — call from `api/features.py` requeue endpoint
  - `notify_feature_rejected(project, feature_id)` — call from `api/features.py` `PATCH` to `closed`
  - `notify_approve_conflict(project, feature_id, detail)` — call from `api/features.py` on 409 approve conflict
  - `notify_board_push_conflict(detail)` — call from `board/writer.py` `commit_and_push_board` on push failure
- Config fields to add to `borealis/borealis/service/config.py`: `telegram_bot_token: str = ""`, `telegram_chat_id: str = ""`

**Aurora** (`aurora/aurora/service/notifications/`)

Mirror the same `telegram.py` + `events.py` structure. Aurora and Borealis send to the same bot/chat; events are distinct and require no coordination.

- `notify_approve_conflict(project, feature_id, detail)` — merge conflict on `aurora approve`; call from `review.py` instead of just logging
- `notify_branch_adoption_failed(project, branch)` — unrelated history on feature branch adoption; call from `git/features.py`
- `notify_invalid_feature_frontmatter(project, feature_id, error)` — bad `_feature.md` during feature setup; call from `git/features.py`
- `notify_oauth_failed()` — Claude auth failure during cloud execution; call from `execution/cloud.py`
- `notify_rate_limit(detail)` — 429 / rate-limited step; reinstated with spend tracking (see Usage Caps section)
- `notify_approaching_soft_cap(spent_usd, soft_cap_usd)` — reinstated with spend tracking (see Usage Caps section)
- Config fields to add to `aurora/aurora/service/config.py`: `telegram_bot_token: str = ""`, `telegram_chat_id: str = ""`

---

#### Usage Caps, Budget Management, and Metric Collection
Full spend tracking and telemetry removed from v1; reimplementation notes follow.

**Budget / cap system:**
- Monthly cloud spend tracked in a JSON file under `aurora_home` (fields: `cycle_start`, `total_usd`; path TBD)
- `SpendTracker` class: `load()`, `reset_if_new_cycle(billing_cycle_day)`, `add_cost(usd)`, atomic write on each update
- Config fields to restore: `monthly_sdk_credit_usd`, `monthly_soft_cap_usd`, `billing_cycle_day`, `max_budget_usd_per_call`
- Agent definition frontmatter field: `max_budget_usd` (per-call ceiling)
- Supervisor soft-cap enforcement: pause runner when `spend.total_usd >= monthly_soft_cap_usd`
- Telegram notifications: `notify_approaching_soft_cap(spent_usd, soft_cap_usd)`, `notify_rate_limit(detail)`
- `/api/status` fields: `monthly_credit_usd`, `monthly_soft_cap_usd`; CLI `status` command displays these
- Rate limit detection: `RateLimitEvent` from Claude Agent SDK → `CloudStepResult(rate_limited=True)` → pipeline returns `QUEUED` for retry

**Metric collection:**
- `TaskState` fields to restore: `input_tokens`, `output_tokens`, `cache_read`, `cache_write`, `estimated_cost_usd`, `model_usage`
- Cloud execution: extract from `ResultMessage.usage` (`input_tokens`, `output_tokens`) and `ResultMessage.cost_usd`; accumulate into `task_state`
- Local execution: extract `prompt_tokens` / `completion_tokens` from Ollama `usage` dict; accumulate into `task_state`
- Longer-term: `task_runs` SQLite table (see Metric Collection section above) storing per-step token counts, cost, model, provider, status, commit hash; daily SQL dump committed to aurora


