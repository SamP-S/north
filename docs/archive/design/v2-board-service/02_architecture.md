## 2. System architecture

### 2.1 Process topology

North runs as the operator's user account on one Linux host, started by systemd
and configured from a single `.env`. The service binds `127.0.0.1` and exposes
REST and MCP on the same port (default `8001`).

```
   ┌──────────────────────────────────────────────────────────────────────┐
   │ north.service (FastAPI / uvicorn)                  bind 127.0.0.1     │
   │  - REST: /api/status, /api/queue, projects, features, tasks,         │
   │          conversations, comments, /api/events (SSE)                  │
   │  - MCP:  /mcp/<grant>/  (board tools over the same service layer)    │
   │  - reads/writes the board in the board repo clone (~/.north/board)   │
   │  - prefixed commits + push to the board repo remote                   │
   └──────────────────────────────────────────────────────────────────────┘
              ▲                                   ▲
        north CLI ── HTTP ──┘            external clients ── HTTP / MCP ──┘
        (127.0.0.1)                      (agent runtime, MCP clients)
```

### 2.2 Single service

`north.service` is one FastAPI process. The REST layer (see
[backend API](06_backend-api.md)) is the primary interface; the MCP surface is
mounted beside it in the same process and calls the same service layer — REST
stays canonical, MCP is a surface. A board-watcher runs as an `asyncio`
background task within the same process: it detects new commits on the board repo
`HEAD`, reloads board state, and resolves the queue (promoting eligible tasks and
detecting feature-review transitions).

The operator interacts with the board directly on the filesystem (editing
Markdown in the board repo) or through the API/CLI.

### 2.3 Source of truth

| Store | Contents | Authority | Writers |
|---|---|---|---|
| Board repo (`~/.north/board`), **main** | all board state, project registry | **source of truth** for the board | North (via API), operator (direct filesystem) |
| Project repo, **main** (`docs/ctx/`) | conventions, architecture, tech stack | **source of truth** for project context | operator (direct filesystem) |
| Project repo (code, feature branches) | project code | source of truth for code | external runtimes, humans |

**If board files and Git disagree, Git wins; the operator repairs the board
manually.**

### 2.4 Board flow

1. `draft` → `ready` — operator or client promotes a task (server-enforced
   draft gate; the feature must be promoted first).
2. `ready` → `queued` — cooldown passes; the resolver picks it up.
3. `queued` → `in_progress` — an external runtime claims the task.
4. `in_progress` → `done` (or `failed` / `blocked`) — the runtime writes the
   result and status back through the API.
5. When all of a feature's tasks reach `done`, the feature moves to `review`.

Every status change is one board commit (`[board:task]` / `[board:feature]`).
Notifications fire on notable transitions.
