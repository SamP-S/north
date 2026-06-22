## 15. Pipelines

### 15.1 Overview

The agentic workflow within a task is defined by a **pipeline** — a YAML file whose name matches the `pipeline` field in the task frontmatter. The pipeline defines a directed graph of agent steps with explicit routing on confidence.

This separates two concerns cleanly:

- **Outer pipeline** (`task_ingest → preflight → branch_setup → agent_prepare → update_board`) — infrastructure; fixed; handled by the plain Python async runner (see [orchestrator](05_orchestrator.md))
- **Inner pipeline** — the agentic workflow; configurable per task via the `pipeline` field in the task frontmatter

**Resolution:** `pipeline: map-code-review` resolves to `definitions/pipelines/map-code-review.yaml`. Project-specific overrides in `projects/{project}/pipelines/` take precedence over globals by name — same pattern as agent definitions (see §6.2 in [agent execution](06_agent-execution.md)). The runner checks the project directory first, falls back to globals.

Each task references a pipeline by name. The runner loads the pipeline file, executes the graph starting at the `entry` step, and feeds the terminal artifact into `update_board`.

### 15.2 Pipeline schema

A pipeline file has three top-level fields:

```yaml
name: <string>     # must match the filename (without .yaml)
entry: <step id>   # id of the first step to execute; must reference a valid step id
steps: [...]       # ordered list of step definitions (order is for readability only; entry determines execution start)
```

The graph validator enforces that `entry` references an existing step id and that all routing targets (`confidence.*`, `on_fail`) reference existing step ids or the reserved values `stop`/`done`.

### 15.3 Step schema

Every step in a pipeline has the same shape:

```yaml
- id: coder                  # unique within the pipeline; used as a goto target
  agent: basic_coder         # references an agent definition by name
  max_attempts: 3            # retries this step on artifact parse failure before following on_fail
  confidence:                # first-class routing based on the agent's confidence output
    high: reviewer
    medium: code_checker
    low: code_checker
    blocked: stop
  on_fail: stop              # followed when parse-failure attempts are exhausted
```

Required fields: `id`, `agent`, `confidence` (all four levels: `high`, `medium`, `low`, `blocked`), `on_fail`. Optional field: `max_attempts` (defaults to `1`). A pipeline is only loaded if every step passes schema validation — missing required fields cause the pipeline to be rejected at load time and the task set to `status: blocked`.

### 15.4 Step execution contract

For each step:

1. Run the agent; it produces an **artifact** (§15.6)
2. If the artifact is malformed (missing frontmatter, invalid YAML, missing or invalid `confidence`) — count as an attempt; inject parse error as context; retry if attempts remain; follow `on_fail` if exhausted
3. Route via `confidence` (`high`, `medium`, `low`, or `blocked`)

Quality gates (linting, tests, review) are expressed as dedicated agent steps in the pipeline, not as engine-level checks. Those agents use Bash tools to run tooling, interpret the results, and return an appropriate confidence level to route back or forward.

### 15.5 Routing targets

Any `confidence.*` or `on_fail` value is one of:

| Value | Meaning |
|---|---|
| `<step id>` | Jump to that step |
| `stop` | Fail the task; notify operator |
| `done` | Pipeline complete; proceed to `update_board` |

### 15.6 Artifacts

The pipeline maintains a single ordered artifact chain in `TaskState`. `task_ingest` produces artifact `[0]` from the task file, seeding the chain; every subsequent agent step appends to the chain. All prior artifacts are passed as context to each step.

**Context window note:** in long pipelines with retries, the accumulated artifact text can grow large. For cloud steps this is rarely an issue; for local steps, the assembled prompt must fit within the agent's `num_ctx` (see §11.1) — Ollama will truncate silently if it does not. Pipeline authors targeting local steps should keep artifact bodies concise and use the `num_ctx` agent field to tune the context window accordingly. A `max_context_artifacts` limit is a planned feature (see [planned features](99_planned-features.md)).

Every artifact has the same shape:

```markdown
---
agent: basic_coder
confidence: low
status: complete
summary: "Implemented auth token validation in src/auth.py"
---

[agent's full output here]
```

The agent's role prompt instructs it to format its entire response as a Markdown artifact with the required frontmatter. The runner parses the frontmatter to extract `confidence` for routing; the body is passed as context to subsequent steps and written to `{id}-{slug}.result.md` by `update_board` (terminal step only). If the agent's response is malformed (see §6.1), the parse failure is treated as a step failure and retried.

Required fields: `agent`, `confidence`, `status`, `summary`. Valid `confidence` values: `high`, `medium`, `low`, `blocked`. Valid `status` values: `complete`, `failed`, `blocked` — distinct from the task state machine values.

The task artifact (`[0]`) uses `agent: system, confidence: high, status: complete` and contains the task id, title, and body as its content.

Cross-reference: §15.4 describes how the runner reads and routes on `confidence`; §15.5 lists valid routing targets.

### 15.7 Quality gates

Quality gates (linting, type checking, tests, review) are expressed as dedicated agent steps in the pipeline — not as engine-level checks. A `qa_checker` agent uses Bash tools to run the project's toolchain (e.g. `uv run ruff check .`, `uv run pytest`), interprets the results, and returns confidence `high` (all pass) or `low` (failures found) to route accordingly.

This keeps the pipeline language-agnostic and the engine simple. Aurora ships global template pipelines in `definitions/pipelines/` as starting points; project overrides live in `projects/{project}/pipelines/`.

### 15.8 Agent definitions and pipelines

Agent definitions (see §6.2 in [agent execution](06_agent-execution.md)) carry model, prompt, tools, and permissions. Pipelines carry topology only. The two are fully decoupled — the same agent can appear at multiple steps in a pipeline, and the same pipeline can be used across different projects with different agent overrides.

Escalation is expressed as additional steps referencing purpose-built agents (e.g. `sonnet_coder`, `opus_coder`) with prompts tuned for their respective models. There is no `model` override on a step.

### 15.9 Example pipeline

```yaml
name: map-code-review
entry: mapper
steps:
  - id: mapper
    agent: basic_mapper
    max_attempts: 2
    confidence:
      high: coder
      medium: coder
      low: coder
      blocked: stop
    on_fail: stop

  - id: coder
    agent: basic_coder
    max_attempts: 3
    confidence:
      high: qa_checker
      medium: qa_checker
      low: qa_checker
      blocked: stop
    on_fail: stop

  - id: qa_checker
    agent: qa_checker
    max_attempts: 1
    confidence:
      high: reviewer
      medium: code_checker
      low: code_checker
      blocked: stop
    on_fail: stop

  - id: code_checker
    agent: code_checker
    max_attempts: 1
    confidence:
      high: qa_checker
      medium: sonnet_coder
      low: opus_coder
      blocked: stop
    on_fail: stop

  - id: sonnet_coder
    agent: sonnet_coder
    max_attempts: 2
    confidence:
      high: qa_checker
      medium: qa_checker
      low: stop
      blocked: stop
    on_fail: stop

  - id: opus_coder
    agent: opus_coder
    max_attempts: 1
    confidence:
      high: qa_checker
      medium: qa_checker
      low: stop
      blocked: stop
    on_fail: stop

  - id: reviewer
    agent: reviewer
    max_attempts: 1
    confidence:
      high: done
      medium: done
      low: stop
      blocked: stop
    on_fail: stop
```
