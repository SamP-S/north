## 6. Backend API

### 6.1 Conventions

Base path `/api`; JSON in/out. Auth: none at the REST app layer; uvicorn binds
`127.0.0.1`. Every write is exactly one board commit through the service layer
(the same layer the MCP surface calls). Cross-project reads are app-level routes;
per-project board objects live under `/api/projects/{project}/…`.

### 6.2 Cross-project reads

| Method | Path | Notes |
|---|---|---|
| GET | `/api/health` | Liveness |
| GET | `/api/status` | Service health, runner state, board-loaded flag |
| GET | `/api/projects` | List all registered projects |
| GET | `/api/features` | Active features across all projects; optional `?project=` filter |
| GET | `/api/queue` | Pending and active tasks; optional `?project=` filter; ordered by dependency depth then `ready_at` |
| GET | `/api/review` | Features in `status: review`; optional `?project=` filter |
| GET | `/api/conversations/pending` | Cross-project decomposition queue, oldest first |

### 6.3 Project management

| Method | Path | Notes |
|---|---|---|
| POST | `/api/projects/register` | Body includes `ssh_url`, optional `name`, `base_branch`, `auto_merge`; clones nothing — registers the project, creates board dirs, updates `projects.yaml`, commits `[board:project]`; `409` if the name is already registered |
| GET | `/api/projects/{project}` | Project detail |
| PATCH | `/api/projects/{project}` | Update `base_branch` / `auto_merge` |
| DELETE | `/api/projects/{project}` | Unregister; removes board entry and `projects.yaml` record; response warns of any active features being discarded |

### 6.4 Features (`/api/projects/{project}`)

| Method | Path | Notes |
|---|---|---|
| GET | `/features` | List a project's features |
| GET | `/features/{feature}` | Feature detail |
| POST | `/features` | Create a feature (lands `draft`) |
| PUT | `/features/{feature}` | Edit feature fields |
| PATCH | `/features/{feature}/status` | Transition status (gated by the transition table; `merged`/`closed` recorded here) |
| POST | `/features/{feature}/promote` | `draft → open` |
| POST | `/features/{feature}/requeue` | Re-open a feature, reset tasks to `ready` |
| DELETE | `/features/{feature}` | Delete a feature (draft tasks only) |

### 6.5 Tasks (`/api/projects/{project}`)

| Method | Path | Notes |
|---|---|---|
| GET | `/features/{feature}/tasks` | List a feature's tasks |
| GET | `/features/{feature}/tasks/{task_id}` | Task detail (frontmatter, body, result if present) |
| POST | `/features/{feature}/tasks` | Create a task (lands `draft`) |
| PUT | `/features/{feature}/tasks/{task_id}` | Edit task fields |
| PATCH | `/features/{feature}/tasks/{task_id}/status` | Transition status (gated) |
| POST | `/features/{feature}/tasks/{task_id}/promote` | `draft → ready` (feature must be promoted first) |
| POST | `/features/{feature}/tasks/{task_id}/split` | Replace a task with children atomically |
| DELETE | `/features/{feature}/tasks/{task_id}` | Delete a task |

### 6.6 Conversations and comments (`/api/projects/{project}`)

| Method | Path | Notes |
|---|---|---|
| POST | `/conversations` | Ship a condensed conversation onto the board (`pending`) |
| GET | `/conversations` | List a project's conversations |
| GET | `/conversations/{id}` | Conversation detail (with result) |
| PATCH | `/conversations/{id}/status` | Move forward (`pending → decomposing → decomposed`) |
| GET/POST | `/features/{feature}/comments` | Read / append the feature thread |
| GET/POST | `/features/{feature}/tasks/{task_id}/comments` | Read / append a task thread |

Comment threads are append-only and typed `[question] / [answer] / [note]`; there
are no edit or delete endpoints. Posting an `[answer]` to a task blocked with
`blocked_reason: question` flips it back to `ready` in the same board commit.

### 6.7 MCP surface

The board is also exposed over MCP (streamable HTTP), mounted in the same process
beside REST. One endpoint per grant set, each exposing only its tools:

| Grant | Mount | Tools beyond reads |
|---|---|---|
| `decomposer` | `/mcp/decomposer/` | `create_feature`, `create_task` |
| `implementer` | `/mcp/implementer/` | `add_comment`, `split_task` |
| `reviewer` | `/mcp/reviewer/` | `add_comment`, `promote_draft`, `create_conversation` |
| `cockpit` | `/mcp/cockpit/` | `add_comment`, `promote_draft`, `create_conversation` |

All grants share the read tools. Optional bearer tokens per grant via
`MCP_TOKENS=grant:token,...` (defense-in-depth; the service binds loopback only).
Configure clients with the trailing slash — the bare path answers with a 307
redirect that not every MCP client follows.

### 6.8 Live stream (planned)

An SSE event stream (`GET /api/events`) is planned for live board updates —
board reloads, task/feature status changes, and queue activity. See
[planned features](99_planned-features.md).
