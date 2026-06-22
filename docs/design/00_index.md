# North — Design Specification

North is the git-backed task board service and its companion CLI. It owns board
state — projects, epics, features, tasks, conversations, and comment threads —
and enforces their lifecycle over a REST API and a parallel MCP surface. North
does not execute tasks; an external agent runtime (if any) drives work by talking
to North over HTTP/MCP.

## Sections

| File | Contents |
|---|---|
| [01_overview.md](01_overview.md) | Purpose, goals, and governing principles |
| [02_architecture.md](02_architecture.md) | Process topology, source of truth, board flow |
| [03_repository-layout.md](03_repository-layout.md) | Board repo and project repo layout, runtime dirs |
| [04_board-data-model.md](04_board-data-model.md) | Hierarchy, frontmatter schemas, task state machine, feature lifecycle |
| [05_git-conventions.md](05_git-conventions.md) | Commit prefixes, merge flow, push cadence |
| [06_backend-api.md](06_backend-api.md) | FastAPI conventions, endpoints, SSE, MCP surface |
| [07_cli.md](07_cli.md) | The `north` CLI |
| [08_notifications.md](08_notifications.md) | Telegram outbound events |
| [09_configuration.md](09_configuration.md) | `.env` variables and the `projects.yaml` registry |
| [10_testing.md](10_testing.md) | Unit, integration, and smoke test strategy |
| [99_planned-features.md](99_planned-features.md) | Board-scoped roadmap |
