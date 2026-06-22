# Agent System — Design Specification

## Sections

| File | Contents |
|---|---|
| [01_overview.md](01_overview.md) | Purpose, goals, non-goals, and governing principles |
| [02_key-decisions.md](02_key-decisions.md) | All resolved operator decisions |
| [03_system-architecture.md](03_system-architecture.md) | Process topology, service split, source of truth, and task flow |
| [04_repository-layout.md](04_repository-layout.md) | Repo roles, runtime dirs, and filesystem layout |
| [05_orchestrator.md](05_orchestrator.md) | Async supervisor loop, per-task execution nodes, queue/dependency resolution |
| [06_agent-execution.md](06_agent-execution.md) | Cloud SDK invocation, local Ollama executor, agent definitions, tool permissions, tool definitions |
| [07_board-data-model.md](07_board-data-model.md) | Hierarchy, frontmatter schemas, task state machine, feature lifecycle |
| [08_git-conventions.md](08_git-conventions.md) | Commit prefixes, branch/worktree model, hooks, merge flow, push cadence |
| [09_backend-api.md](09_backend-api.md) | FastAPI conventions, all endpoints, SSE event types, control semantics |
| [10_notifications.md](10_notifications.md) | Telegram outbound events |
| [11_configuration.md](11_configuration.md) | `.env` variables and `projects.yaml` registry format |
| [12_deployment.md](12_deployment.md) | systemd units, Ollama, install script, backup/DR, migrations |
| [13_testing.md](13_testing.md) | Unit, integration, and smoke test strategy |
| [14_build-sequencing.md](14_build-sequencing.md) | Recommended incremental build order with rationale |
| [15_pipelines.md](15_pipelines.md) | Pipeline file format, step schema, artifact contract, execution contract, built-in checks |
| [16_cli.md](16_cli.md) | CLI commands, control semantics, implementation notes |
| [99_planned-features.md](99_planned-features.md) | Roadmap: reconciliation, context agents, memory, metrics, frontend, parallelism |
