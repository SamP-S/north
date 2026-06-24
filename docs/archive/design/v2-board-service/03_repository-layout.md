## 3. Repository and filesystem layout

### 3.1 Repository roles

- **Board repo** — a dedicated Git repo cloned locally (default `~/.north/board`,
  gitignored runtime data); holds all board state and the project registry.
  Configured via `BOARD_REPO_SSH_URL` in `.env` — any operator can point North at
  their own board repo. This is the single source of truth for board state. It has
  its own private remote for DR and is pushed on every board commit. Changes
  frequently — every board mutation and lifecycle event.
- **Project repos** — each carries `CLAUDE.md` and `docs/ctx/`. They contain
  **only code and context** — no board files. Each has its own remote; feature
  branches are pushed by whatever external runtime executes work, so code is
  backed up and portable.

The North package itself (engine code) is installed as a tool; it is not a
board-state store.

### 3.2 Runtime directories (off-Git)

Runtime data lives under `NORTH_HOME` (default `~/.north`, configurable in
`.env`):

```
$NORTH_HOME/
├── board/        # board repo clone — gitignored (configured via BOARD_REPO_SSH_URL)
└── .env          # shared environment file
```

The board clone is the only required runtime directory North owns. Any
execution-side directories (managed clones, feature worktrees) belong to an
external runtime, not to North.

### 3.3 Board repo layout

The board repo is a standalone Git repo cloned to `~/.north/board`. It has its own
private remote for DR and is pushed on every board commit. Its layout:

```
board/
├── projects.yaml                # project registry
└── projects/
    └── {project}/
        ├── conversations/
        │   ├── {id}.md                       # work-intake conversation (frontmatter + body)
        │   └── {id}.result.md                # decomposition result (absent before decomposition)
        └── board/
            ├── epics/
            └── features/
                ├── active/{feature}/
                │   ├── _feature.md
                │   ├── _feature.thread.md     # feature comment thread (append-only)
                │   └── tasks/
                │       ├── {id}-{slug}.md          # task definition (body never modified by North)
                │       ├── {id}-{slug}.result.md   # task result; absent before first run
                │       └── {id}-{slug}.thread.md   # task comment thread (append-only)
                └── archived/{feature}/
```

The board repo has no feature branches — all state lives on its main.

### 3.4 Project repo layout

```
{project}/   (any branch)
├── CLAUDE.md
└── docs/
    └── ctx/                     # free-form; structure is up to the project
```

Project repos carry no board files. `docs/ctx/` is free-form project knowledge —
structure, filenames, and subdirectories are entirely up to the project. The only
requirement is that it lives at `docs/ctx/` so clients can locate it consistently.
