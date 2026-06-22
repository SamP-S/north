## 3. System architecture

### 3.1 Process topology

All processes run as the operator's user account on one Linux host, started by systemd, configured from a single `.env`. Cloud calls authenticate via OAuth credentials stored in Claude Code's credential store by a one-time `claude auth login`; local calls go directly to Ollama at `OLLAMA_BASE_URL`. `ANTHROPIC_API_KEY` must not be set — the SDK will prefer it over OAuth.

```
   ┌──────────────────────────────────────────────────────────────────────┐
   │ aurora.service (FastAPI/uvicorn + async task runner)  bind 127.0.0.1 │
   │  - GET /status, GET /queue, GET /events (SSE), POST /control         │
   │  - async task runner — asyncio.create_task() (concurrency 1)        │
   │  - reads/writes board in the board repo (`aurora/board/`)             │
   │  - prefixed commits + push to board repo remote                       │
   └──────────┬───────────────────────────────┬───────────────────────────┘
              │                               │
  (local: direct HTTP — /api/chat)           (cloud: OAuth → Pro $20/mo credit)
              ▼                               ▼
   ┌──────────────────────┐       ┌──────────────────────┐
   │ Ollama (svc)         │       │ Claude API            │
   │ RTX 3060 / CUDA      │       │                      │
   │ one 7B at a time     │       └──────────────────────┘
   └──────────────────────┘

   aurora CLI  ── HTTP ──►  aurora.service (127.0.0.1)
```

### 3.2 Single service

`aurora.service` combines the API and the runner in one process. The FastAPI layer (see [backend API](09_backend-api.md)) is the IPC surface for the CLI — observation, runner control, and project management. The task runner executes as an `asyncio` background task (`asyncio.create_task()`) within the same process, sharing the event loop with FastAPI. Both share the board repo filesystem directly.

The operator interacts with the board directly on the filesystem (editing Markdown files in the board repo) and monitors or controls the runner via the CLI.

### 3.3 Source of truth

| Store | Contents | Authority | Writers |
|---|---|---|---|
| Board repo (`aurora/board/`), **main** | all board state, project registry, project-specific agent/pipeline overrides | **source of truth** for the board | runner, operator (direct filesystem) |
| aurora, **main** (`definitions/agents/`) | global agent definitions | **source of truth** for global agents | operator |
| aurora, **main** (`definitions/pipelines/`) | global pipeline definitions | **source of truth** for global pipelines | operator |
| aurora, **main** (`definitions/tools/`) | global tool definitions (local agents) | **source of truth** for local agent tools | operator |
| aurora repo | engine code | source of truth for the engine | operator |
| Project repo, **main** (`docs/ctx/`) | conventions, architecture, tech stack | **source of truth** for project context | operator (direct filesystem) |
| Project repo (code, feature branches) | project code | source of truth for code | agents (in worktrees), humans |

**If board files and Git disagree, Git wins; the operator repairs the board manually.**

### 3.4 Task flow

1. `draft` → `ready` — operator marks task ready (edits file directly)
2. `ready` → `queued` — cooldown passes; resolver picks it up
3. `queued` → `in_progress` — runner claims the task
4. Graph executes: ingest → preflight → branch/worktree setup → assemble agents + context → run pipeline → update board
5. `in_progress` → `done` (or `failed` / `blocked`)

The agent's code edits commit to the project feature branch (`[agent:*]`); board status/output updates commit separately to the board repo (`[system:task]`). Telegram fires on notable transitions. The runner loops.
