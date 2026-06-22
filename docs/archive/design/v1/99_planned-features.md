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
- **Daily backup:** integrity check → SQL dump → committed to aurora as `data/metrics.sql`

#### Frontend UI
A web-based board interface served by the aurora service.
- Project epic/feature/task timeline tree/graph
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

