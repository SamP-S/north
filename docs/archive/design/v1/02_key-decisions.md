## 2. Key decisions

Resolved with the operator:

1. **Board location** — board state, project registry, and project-specific agent/pipeline overrides live in a **dedicated board repo** (separate from aurora). Aurora holds engine code and global agent/pipeline definitions only. Project repos carry no board files. **Global agent definitions** in aurora (`definitions/agents/`); **global pipeline definitions** in aurora (`definitions/pipelines/`). **Project-specific overrides** and **all board state** in the board repo (`projects/{project}/agents/`, `projects/{project}/pipelines/`, `projects/{project}/board/`). **Context** (`docs/ctx/`) stays in the project repo — it's project knowledge readable by agents from the worktree and useful to human contributors. See [repository layout](04_repository-layout.md).
2. **Local execution** — Local models are served by Ollama and invoked directly via Aurora's `local_executor` using Ollama's native `/api/chat` endpoint. Aurora implements its own multi-turn tool execution loop; tool definitions live in `definitions/tools/` as JSON files (Ollama/OpenAI function calling format) and are referenced by name in agent definitions. Cloud agents use the Claude Agent SDK exclusively. LiteLLM is not used. 7B unreliability mitigated by small-task decomposition, confidence escalation, and retry on malformed tool calls. See [agent execution](06_agent-execution.md).
3. **Concurrency** — one task at a time (`RUNNER_CONCURRENCY=1`).
4. **Model per agent** — each agent definition declares its own model; no global escalation model. Escalation is expressed as purpose-built agents (e.g. `sonnet_coder`, `opus_coder`) in the pipeline graph (see [pipelines](15_pipelines.md)).
5. **Pipelines** — the agentic workflow within a task is defined by a YAML pipeline file in `definitions/pipelines/`. Tasks reference a pipeline by name. The pipeline defines a graph of agent steps with per-step confidence routing, binary checks, retry limits, and explicit escalation paths. See [pipelines](15_pipelines.md).
6. **Auth** — Claude subscription via **OAuth** (Pro), using the **$20/month Agent SDK credit**; no API key for cloud.
7. **Cloud-usage control** — monthly-credit soft cap + `RateLimitEvent` + usage-credits-disabled hard stop; per-call `max_budget_usd` + `max_turns`.
8. **Agent integration** — Python `claude-agent-sdk`, **non-bare**, agents passed **programmatically**.
9. **Permission** — `acceptEdits` + scoped tools per agent; Bash always scoped + global deny list.
10. **Per-agent config** — see [agent execution](06_agent-execution.md).
11. **Feature review gate** — when all tasks are `done` the runner sets feature `status: review` and stops. The operator reviews the diff locally then runs `aurora approve <feature>` (merge + archive), `aurora rollback <feature>` (reset branch + tasks, retry), or `aurora reject <feature>` (abandon). No GitHub API required. Feature branches are pushed throughout for code backup.
12. **CLI live updates** — SSE via `/api/events`.
13. **Stack** — Python 3.12; `uv` for package management and virtualenv; `ruff` for linting/formatting; `mypy` for type checking; `pytest` for tests; `pydeps` for dependency graphs.
14. **Notifications** — Telegram, outbound only (incl. OAuth-failure alert).
15. **Access** — CLI connects to `aurora.service` on `localhost`.
