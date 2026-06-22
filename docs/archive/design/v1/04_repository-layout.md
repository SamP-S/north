## 4. Repository and filesystem layout

### 4.1 Three repository roles

- **`aurora/`** — the orchestration engine; lives on the server permanently; holds engine code, global agent definitions, and global pipeline definitions. Has its own private remote for DR. Changes infrequently — only when the engine or global agent/pipeline definitions change.
- **Board repo** (e.g. `borealis`) — a separate Git repo cloned to `aurora/board/` (gitignored); holds all board state, the project registry, and project-specific agent/pipeline overrides. Configured via `BOARD_REPO_SSH_URL` in `.env` — any user can point aurora at their own board repo. This is the single source of truth for board state. Changes frequently — every task completion, operator board edit, and feature lifecycle event.
- **Project repos** — each carries `CLAUDE.md` and `docs/ctx/`. They contain **only code and context** — no board files, no agent definitions. Each has its own remote; feature branches are pushed so code is backed up and portable.

### 4.2 Runtime directories (off-Git, outside the aurora repo)

All runtime directories live under `AURORA_HOME` (default `~/.aurora`, configurable in `.env`):

```
$AURORA_HOME/
├── repos/{project}              # managed clone per project (main checked out; base for worktrees)
├── worktrees/{project}/{feature} # one worktree per active feature branch (code only)
└── data/
    └── spend.json               # billing-cycle spend counter (see §5.9 in orchestrator)
```

These directories are outside the aurora repo entirely and never tracked by Git.

### 4.3 `aurora/` layout

```
aurora/
├── definitions/
│   ├── agents/                  # global agent definitions (one .md per agent)
│   ├── pipelines/               # global pipeline definitions (one .yaml per pipeline)
│   ├── tools/                   # local model tool definitions (one .json per tool, Ollama function calling format)
│   └── templates/
│       └── tasks/               # task file templates (one .md per template; operator copies and customises)
├── service/                     # aurora.service — FastAPI app + async task runner (one process)
├── cli/                         # aurora CLI entry point
├── scripts/                     # install and setup helpers
├── tests/                       # unit and integration tests
├── systemd/                     # service unit files
└── board/                       # board repo clone — gitignored (configured via BOARD_REPO_SSH_URL)
```

Global agent, pipeline, and tool definitions live in `definitions/` — one file per definition. New definition types (e.g. check definitions) can be added as subdirectories as the system evolves. Examples of agents: `mapper`, `coder`, `reviewer`, `doc_writer`. Tool definitions are JSON files used exclusively by local agents; cloud agents use the Agent SDK's native tool system (see [agent execution](06_agent-execution.md)).

### 4.4 Board repo layout

The board repo (e.g. `borealis`) is a standalone Git repo cloned to `aurora/board/`. It has its own private remote for DR and is pushed on every board commit. Its layout:

```
board/
├── projects.yaml                # project registry
└── projects/
    └── {project}/
        ├── agents/              # project-specific agent overrides (override globals by name)
        ├── pipelines/           # project-specific pipeline overrides (override globals by name)
        └── board/
            ├── epics/
            └── features/
                ├── active/{feature}/
                │   ├── _feature.md
                │   └── tasks/
                │       ├── {id}-{slug}.md          # operator task definition (body never modified by runner)
                │       └── {id}-{slug}.result.md   # runner output; overwritten each run; absent before first run
                └── archived/{feature}/
```

Project-specific overrides live alongside the board they belong to. The board repo has no feature branches — all state lives on its main.

### 4.5 Project repo layout

```
{project}/   (any branch)
├── CLAUDE.md
└── docs/
    └── ctx/                     # free-form; structure is up to the project
```

Project repos carry no board files and no agent definitions. Worktrees (`$AURORA_HOME/worktrees/{project}/{feature}`) check out a project feature branch for code execution; the agent reads `docs/ctx/` from the worktree naturally. Board reads and writes go to the board repo (`aurora/board/`) directly.

`docs/ctx/` is free-form project knowledge — structure, filenames, and subdirectories are entirely up to the project. This pairs with project-specific agent and pipeline overrides in the board repo, allowing each project to define its own context shape and the agents best suited to read it. The only requirement is that it lives at `docs/ctx/` so agents can locate it consistently.
