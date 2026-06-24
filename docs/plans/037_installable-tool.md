# Plan: North MVP — a Backlog.md-style, in-repo Markdown task board + CLI (+ optional MCP)

## Context

North began as a live service (FastAPI daemon, systemd, a `projects → features → tasks` board in
`~/.north/board`, a rigid 8-state machine, cooldown/queue, REST + per-grant MCP). We are
collapsing it to an **installable tool over a single, in-repo Markdown task board**, modeled on
**Backlog.md**: the board lives in a `north/` directory inside the user's own project repo,
tasks are plain `.md` files, and **git is the user's responsibility** (no auto commit/push).

`north <cmd>` operates directly on the files — no daemon, nothing in `~/.north` (minimal/none).
An optional single MCP server can be started on demand for agents.

### Decisions locked with the user
1. **In-repo `north/` board**, North-branded but mirroring Backlog.md's layout. Discovered by
   walking up from cwd (like `.git`). Minimal-to-nothing in `~/.north`.
2. **Tasks only** — no projects, features, conversations, comments, or splitting. Decomposition is
   the user's job via `depends_on` + notes in the task body.
3. **6-state machine** (below). No cooldown, no `queued`, no `superseded`.
4. **Lifecycle by directory** — each task file lives in the folder for its status; changing status
   moves the file. The folder is the source of truth.
5. **Schema cuts** — drop `blocked_reason`, `split_from`, `decomposed_from`, `ready_at`,
   `pipeline`-as-required. Blocker details live in the body.
6. **`agent` field** (renamed from `pipeline`) — a free-form string the user namespaces with their
   own delimiter to encode agent/provider/executor, e.g. `opus4.8`, `aurora:hard-coding-pipe`,
   `ollama:llama3.2`. North treats it as opaque text.
7. **No REST**; logic in a plain `north/core/`; **single MCP** at `http://127.0.0.1:<mcp_port>/mcp`,
   on-demand via `north mcp start|stop|status|run`.
8. **`north/config.yml`** is the board-discovery marker (walk up for the file, not a bare dir) and
   holds per-board behaviour. MVP fields: `mcp_port` (default 8001) and `auto_commit` (default
   `false`). Statuses stay **hardcoded** for now (see Future work); `MCP_TOKEN` stays an env secret
   (never committed in config).
9. **`auto_commit`** (config, default `false`): when `true`, North runs a local `git add` +
   `git commit` of the changed `north/…` files after each mutation; when `false`, it only
   writes/moves files and leaves git to the user. No push/pull/rebase either way.

---

## Board layout (inside the user's repo)

```
<project-repo>/
  AGENTS.md                         # how agents drive North (written by `north init`)
  north/
    config.yml                      # board marker + per-board settings
    draft/        ready/        in_progress/
    done/         failed/       blocked/
      task-12 - Add-login-form.md
    archive/                        # tasks removed from the active board (orthogonal to status)
  ...the project's own code...
```
- `north/config.yml` (MVP): `mcp_port: 8001`, `auto_commit: false`. It is the discovery anchor.
- One Markdown file per task, `task-<n> - <Title-slug>.md` (Backlog.md naming, `task-` prefix).
- Status == the folder the file sits in. `north init` scaffolds `config.yml` + the six folders.
- Changes are ordinary file edits/moves; with `auto_commit: false` (default) they show up in
  `git status` for the user to commit; with `auto_commit: true` North commits each change locally.
  Either way North never pushes/pulls.

### Task schema (frontmatter + body)
```yaml
id: task-12
title: Add login form
status: ready            # mirrors the folder (folder is source of truth), kept in sync
agent: opus4.8           # optional, free-form, opaque to North (may be empty)
labels: [auth, backend]  # free-form list of strings; user adds/removes at will
depends_on: [task-4]     # task ids
created_at: '2026-06-24'
updated_at: '2026-06-24' # auto-bumped on every create/edit/move
```
Body = description + any blocker notes / context as organised free text. The body is treated as
an open encapsulation of task-specific content the user shapes with their own templates (change
logs, directory maps, todos, acceptance, testing) — North does not impose structure on it.

### State machine (6 states)
```
draft ──promote──▶ ready ──start──▶ in_progress ──▶ done
                    ▲                      ├──▶ failed ──┐
                    └──────resolve─────────┴──▶ blocked ─┘
```
- `draft → ready` (human gate). `ready → in_progress`. `in_progress → done | failed | blocked`.
- `failed → ready`, `blocked → ready` (user resolves). `done → ready` is allowed too — edit the
  body with further changes and send it back to `ready` to be reprocessed/continued.
- Transitions are lightly gated (illegal jumps rejected); status changes move the file between
  folders.

---

## Target architecture

```
north/
  core/                    # plain Python, file I/O (+ optional local git commit); no FastAPI/HTTP
    errors.py              # BoardError + NotFound/Conflict/Invalid
    board.py               # locate_board() (walk up for north/config.yml), init_board(), load_config(), next_id()
    tasks.py               # create / get / list / edit / move / archive / cleanup / delete
    git.py                 # commit_board(paths) — only used when config.auto_commit
    instructions.py        # AGENTS.md / `north instructions` text (single source)
  service/                 # single MCP server only
    main.py                # FastAPI app mounting ONE MCP server at /mcp
    mcp.py                 # ~5 tools call north.core.*; BoardError → ValueError
    config.py              # MCP_TOKEN (env secret); mcp_port comes from the board config.yml
    logsetup.py
  cli/
    commands/{init,task,board,cleanup,instructions,mcp}.py
    main.py                # subcommands: init, task, board, cleanup, instructions, mcp
    errors.py              # CLIError
```
**Deleted wholesale:** `service/api/`, `service/orchestrator/`, `service/events.py`,
`service/notify.py`, `service/board/{loader,parser,writer}` collapse into `core/` (frontmatter
read/write + file moves), `service/models.py` shrinks to `TaskStatus` + a `Task` dataclass.
Gone with them: the supervisor, cooldown/queue resolver, git pull/push/rebase + `sync_remote`,
the cross-process board lock (board is now ordinary repo files), projects/features/conversations/
comments, REST, per-grant MCP, notifications, the `~/.north/board` clone.

### Core specifics
- `locate_board()`: from cwd, walk up until a `north/config.yml` is found; error with a clear hint
  if none (`run north init`). `init_board()`: write `config.yml`, the six status folders + `archive/`,
  and an `AGENTS.md` at repo root (from `instructions.py`). `load_config()`: parse `config.yml`
  (`mcp_port`, `auto_commit`).
- `next_id()`: `max` over `task-<n>` across all folders (incl. `archive/`), `+1`.
- `move_task(id, new)` (CLI `task move`): validate against the **hardcoded** transition table,
  rewrite the `status:` frontmatter mirror, and `move` the file to the new folder (source of truth
  = folder; frontmatter kept in sync).
- `archive_task(id)` / `cleanup()`: move a task (or all `done/`) into `archive/`. **Archive is
  orthogonal to status** — an archived file keeps its last `status:` in frontmatter (read from
  there, not the folder) and is excluded from `board` and default `list` (use `--archived`).
- `updated_at` is stamped equal to `created_at` on create and bumped on every `edit`/`move`/`archive`.
- Every mutation (`create`/`edit`/`move`/`archive`/`cleanup`/`delete`) finishes with: if
  `config.auto_commit`, `git.commit_board(changed_paths)`; otherwise leave the working tree dirty for
  the user. No push/pull/rebase. No cross-process lock for MVP (a light `fcntl` lock can be added
  later if concurrent CLI+MCP writes become a concern).

### Single MCP surface
One `FastMCP("north")` at `/mcp`, one optional `MCP_TOKEN` (env). Tools (call `north.core.tasks`):
`list_tasks`, `get_task`, `create_task`, `set_task_status`, `edit_task`. `north mcp` manages it
(detached uvicorn bound to `config.mcp_port`; PID/logs under `~/.north/`, the only `~/.north` usage).

### CLI
- `north init` — scaffold `north/config.yml` + status folders + `archive/`, and write `AGENTS.md`.
- `north task create <title> [--agent A] [--labels ...] [--depends-on ...] [--body|--body-file]` →
  lands in `draft/`.
- `north task list [--status S] [--archived] [--plain|--json]` — list/filter tasks.
- `north task view <id> [--plain|--json]` — show one task (frontmatter + body).
- `north task edit <id> [--title --agent --labels --depends-on --body|--body-file]` — edit fields/body.
- `north task move <id> <status>` — the single state-setter: validates the transition and moves the
  file between folders. Replaces the old `task status`/`promote` (draft gate = `task move <id> ready`).
- `north task archive <id>` — move a task into `archive/` (off the active board).
- `north task delete <id>` — delete a task.
- `north board` — board summary, counts per status.
- `north cleanup [--older-than DAYS]` — bulk-archive `done/` tasks to keep the board readable.
- `north instructions` — print the agent guidance (same text written to `AGENTS.md`).
- `north mcp start|stop|status|run` — manage the on-demand MCP server.
- **Agent-friendly output:** `--plain` (stable unformatted text) and `--json` (structured) on
  `task list` / `task view`; default human output otherwise. North is agent-driven, so machine-
  readable output is first-class.
- All commands run in-process over the discovered `north/` board; no service needed.

---

## Tests
- **Delete** everything tied to removed surfaces: `test_api`, `test_projects_api`,
  `test_conversations`, `test_comments`, `test_gate_events`, `test_refine_rule`,
  `test_archive_clean`, `test_resolver`, `test_split`, `test_git_watcher`, `test_notify`,
  and CLI `test_cli_{projects,feature,conversation,comment,lifecycle}`.
- **Rewrite** against a tmp repo with a `north/` board: `test_parser`→`test_core_tasks`,
  `test_draft_gating`→state-machine tests, `test_mcp` (single `/mcp`, ~5 tools),
  `test_cli_{task,board}`, `test_startup`→`test_core_board` (locate/init/next_id), `conftest.py`.
- **Add** `test_core_board.py` (locate walks up; init scaffolds config/folders/archive/AGENTS.md;
  move relocates the file), `test_core_archive.py` (archive/cleanup move to `archive/`, excluded from
  active views, status read from frontmatter), `test_cli_init.py`, `test_cli_mcp.py`,
  `test_cli_output.py` (`--plain`/`--json` shape; `updated_at` bumps), `test_cli_instructions.py`.

---

## Documentation & old-doc removal
The current `docs/design/` (00–10 + 99) documents the obsolete board-service. Following the `036`
precedent (which archived v1 under `docs/archive/design/v1/`):
- `git mv docs/design → docs/archive/design/v2-board-service/` (archive as history; do **not** delete).
- Author a fresh, smaller `docs/design/`: `00_index`, `01_overview`, `02_board-data-model`,
  `03_cli`, `04_mcp`, `05_configuration`, `06_testing`.
- Rewrite `README.md` + `CLAUDE.md` for the in-repo board tool.
- Untouched history: `docs/plans/**` (this is `037`), `docs/archive/**`, `docs/research/**`; leave
  the untracked `docs/reference/**`.

## Ordered todo
- [x] 1. Save this plan into the repo as `docs/plans/037_installable-tool.md` (working copy lived at
       `~/.claude/plans/synthetic-chasing-kettle.md`; keep `037` updated as implementation proceeds).
- [x] 2. **Core:** `errors.py`; `board.py` (locate/init/next_id); slim `models` (TaskStatus + Task);
       `tasks.py` (frontmatter read/write + file moves; transition table).
- [x] 3. **MCP:** rewrite `mcp.py` (one `/mcp`, ~5 tools, single token); `main.py` mounts it.
       Delete `service/api/`, `orchestrator/`, `events.py`, `notify.py`, old board/loader/parser/writer.
- [x] 4. **CLI:** `commands/{init,task,board,cleanup,instructions,mcp}.py` (incl. `task move/view/
       archive`, `--plain`/`--json`, `--archived`); `main.py` subcommands; delete dead command +
       client modules.
- [x] 4b. **Backlog.md parity adds:** `updated_at` (bump on edit/move/archive); `archive/` +
       `archive_task`/`cleanup`; `core/instructions.py` + `AGENTS.md` write at init + `north
       instructions`; agent-friendly `--plain`/`--json` rendering.
- [x] 5. **Config/scripts/systemd:** `north/config.yml` schema (`mcp_port`, `auto_commit`) +
       `load_config()`; `core/git.py` (auto-commit); `service/config.py` → `MCP_TOKEN` only;
       `install.sh` → `uv tool install .` only; delete `systemd/north.service`.
- [x] 6. **Tests:** delete/rewrite/add per above.
- [x] 7. **Docs:** archive old `docs/design/` → `docs/archive/design/v2-board-service/`; write the
       fresh `docs/design/` set; rewrite README.md + CLAUDE.md (see "Documentation & old-doc removal").
- [x] 8. **Verify** (below); fix lint/type/test fallout.

## Change history
- [2026-06-23] Drafted (installable tool, function core, drop REST, derived cooldown).
- [2026-06-24] Cuts: conversations, comments (→body), cockpit + per-grant MCP (single `/mcp`),
  then features and projects (flat board).
- [2026-06-24] Reframe to **Backlog.md model**: in-repo `north/` board; drop split, cooldown/queue,
  `blocked_reason`/`split_from`/`decomposed_from`/`ready_at`; 6-state machine; lifecycle by folder;
  rename `pipeline`→`agent` (free-form); git is the user's responsibility (no auto push/pull).
- [2026-06-24] Add `north/config.yml` (board marker; `mcp_port`, `auto_commit`); add `labels`
  (free-form list); `agent` optional; `done → ready` reopen allowed; statuses stay **hardcoded**
  (configurable statuses deferred — see Future work).
- [2026-06-24] CLI cleanup: board summary `north board` (was `north status`); single state-setter
  `north task move <id> <status>` (replaces `task status` + `promote`); `task view` (was `show`).
- [2026-06-24] Backlog.md parity adds for the MVP: `updated_at` field; agent-friendly
  `--plain`/`--json` output; `AGENTS.md` + `north instructions` (agent guidance); `archive/` +
  `north task archive` / `north cleanup`.
- [2026-06-24] Plan approved and persisted to `docs/plans/037_installable-tool.md`.
- [2026-06-24] Implemented end-to-end. Built `north/core/` (errors, models, board, tasks, git,
  instructions); rewrote `north/service/` to a single MCP server at `/mcp` (config, logsetup,
  main, mcp) and deleted `api/`, `orchestrator/`, `events.py`, `notify.py`, `board/`,
  `models.py`, `startup.py`; rewrote `north/cli/` (init, task, board, cleanup, instructions, mcp;
  render helpers) and deleted the HTTP client + old command modules; `install.sh` → `uv tool
  install`; removed `systemd/`; pruned pyproject deps (dropped httpx, sse-starlette, pydantic;
  kept pydantic-settings for MCP_TOKEN). Tests rewritten against a tmp board. Docs: archived old
  `docs/design/` → `docs/archive/design/v2-board-service/`, wrote new `docs/design/` set, rewrote
  README + CLAUDE.md.
  Verified: `ruff` clean, `mypy north` clean (strict), 32 tests pass; manual e2e (init → create →
  move → board → cleanup), `north mcp start/status/stop` (health 200), and `auto_commit` all OK.
  Deviations from the plan: task model lives in `north/core/models.py` (not `service/models.py`);
  added `north/cli/render.py` for `--plain`/`--json` rendering; `service/config.py` keeps
  pydantic-settings for the `MCP_TOKEN` env var.

---

## Verification
1. `uv sync` / `uv run ruff check .` clean; `uv run mypy north` no new errors; `uv run pytest` passes.
2. **End-to-end in a tmp repo:** `north init` creates `north/config.yml` + six folders → `north
   task create "Add login" --agent opus4.8` lands `north/draft/task-1 - Add-login.md` → `north
   task move 1 ready` moves it to `ready/` → `north task move 1 in_progress` → `north task move 1
   done` moves it to `done/`; `north task edit 1 --body` records a blocker note; `north board` shows
   the per-status counts. With default `auto_commit: false`,
   `git status` shows the changes uncommitted; flipping `auto_commit: true` makes each op a local
   commit (and still no push).
3. **Discovery:** running `north task list` from a subdirectory finds the board by walking up.
4. **MCP on demand:** `north mcp start` → an MCP client reaches `http://127.0.0.1:8001/mcp` and
   lists the task tools → `north mcp stop`.
5. **Tool install:** `uv tool install .` → `north --help` works with nothing running.
6. **Grep gate:** no `project`/`feature`/`conversation`/`comment`/`cooldown`/`queue`/`supervisor`/
   `split`/`blocked_reason`/`httpx`(in cli)/`/api/` in active `north/` code.

### Settled
- Frontmatter `status:` mirror kept and synced to the folder. `agent` optional. `done → ready`
  reopen allowed. `labels` (free-form list of strings) added; no other extra fields and no imposed
  body structure. `north/config.yml` is the discovery marker and holds `mcp_port` + `auto_commit`;
  statuses + `task-` prefix hardcoded for the MVP; `MCP_TOKEN` stays an env secret.
- Backlog.md parity folded into the MVP: `updated_at`; `--plain`/`--json` agent output; `AGENTS.md`
  + `north instructions`; `archive/` + `north task archive` / `north cleanup`. Deferred to Future
  work: terminal kanban TUI, fuzzy `search`, `export`, shell completion, `assignee`, `priority`,
  acceptance-criteria/DoD, `doc`/`decision`/`milestone`/`sequence`, sub-tasks, cross-branch checks.

## Future work (deferred, not MVP)
- **Configurable statuses** — promote the hardcoded 6-state set + transition rules into
  `north/config.yml` (`statuses`, `default_status`), Backlog.md-style, so projects can define their
  own columns/workflow. Investigate the transition-enforcement model (free `any → any` vs a
  retained draft gate) at that time. Likewise `task_prefix` and a possible `on_status_change` hook
  (the seam to auto-launch the task's `agent`).
