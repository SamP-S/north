# North

A Python monorepo containing two background services that together form an agentic development assistant.

- **Aurora** — agentic execution engine. Picks up tasks from the board, runs each through a linear pipeline of opencode sessions and deterministic gates, and manages git worktrees for each feature.
- **Borealis** — board state manager. Owns a git-backed task board, exposes a REST API, and handles task/feature lifecycle state. Aurora talks to Borealis over HTTP.

Both run as systemd user services and are managed via their respective CLIs.

---

## Requirements

- Python 3.12+
- [uv](https://docs.astral.sh/uv/)
- [opencode](https://opencode.ai/) server (the agent runtime; installed by `install.sh`)
  with at least one configured provider (a subscription provider and/or
  [Ollama](https://ollama.com/) for local models)
- A git repo to use as the task board (SSH access required)
- **Optional — [Ollama](https://ollama.com/) for local models.** North does *not*
  install or manage ollama; if a pipeline uses a local model, run ollama yourself
  (point opencode's ollama provider, and `OLLAMA_URL`, at it — default
  `http://127.0.0.1:11434`). When ollama is needed but unreachable, the affected
  tasks defer (stay `queued`) and auto-resume when it returns; cloud-only pipelines
  are unaffected.

---

## Install

**1. Set required environment variables**

Create `~/.north/.env`:

```env
BOARD_REPO_SSH_URL=git@github.com:your-org/your-board-repo.git
```

Optional overrides:

```env
AURORA_PORT=8000
BOREALIS_PORT=8001
NORTH_HOME=~/.north
AURORA_HOME=~/.north/aurora
BOARD_PATH=~/.north/borealis/board
```

**2. Run the install script**

```bash
bash scripts/install.sh
```

This will:
- Install Python dependencies via `uv sync`
- Install and authenticate the Claude Code CLI
- Create `~/.north/aurora/{repos,worktrees}` directories
- Clone the board repo into `~/.north/borealis/board/`
- Install the pinned opencode CLI (agent runtime server)
- Install and enable `aurora.service`, `borealis.service`, and `opencode.service` as systemd user units (ollama is not managed by North — see Requirements)
- Run an OAuth smoke test against Claude

**Headless / CI install:**

```bash
HEADLESS=1 CLAUDE_CODE_OAUTH_TOKEN=<token> bash scripts/install.sh
```

---

## Services

| Service   | Default port | Logs                                              |
|-----------|-------------|---------------------------------------------------|
| Aurora    | 8000        | `journalctl --user -u aurora.service -f`          |
| Borealis  | 8001        | `journalctl --user -u borealis.service -f`        |
| Opencode  | 4096        | `journalctl --user -u opencode.service -f`        |

## Session execution

Aurora keeps the deterministic outer loop (queue, worktrees, commits,
statuses); everything inside a session belongs to the agent runtime
(`opencode serve`, `OPENCODE_URL`, default `http://127.0.0.1:4096`).

### Session profiles

`aurora/definitions/profiles/<name>.md` — markdown frontmatter + body. The
frontmatter picks the seat's `model`, optional `provider` (defaults to
`OPENCODE_LOCAL_PROVIDER`, default `ollama`), `denied_tools` (added to a
global denylist; the harness owns commits, so `git commit`/`git push` are
denied), and an optional `tools:` map of opencode tool patterns to booleans
(per-seat grants — e.g. the decompose profile enables `borealis_*` MCP
tools, which are disabled globally in `opencode.jsonc` so other seats never
see them). The body is the session's system prompt and ends by requesting a
handoff note (`## Summary / ## Decisions / ## Concerns / ## Suggested
status`) — requested as a courtesy for humans and later sessions, never
parsed for routing.

### Pipelines

`aurora/definitions/pipelines/<name>.yaml` — an ordered, strictly linear
stage list; a task selects one via its `pipeline` field. Two stage types:

```yaml
name: default
stages:
  - run: {profile: implement}
  - gate: {checks: [build, lint, test], policy: if-present}
  - run: {profile: review}
  - gate: {checks: [build, lint, test], policy: if-present}
```

No routing keys, no branching — control flow lives in the harness. After
each successful stage the harness commits a dirty worktree as
`[task:<id>][<stage>] <summary>`. Session failures retry once; rate limits
requeue the task; auth failures block it; a failed gate fails it.

### Gates and the checks manifest

Pipelines name abstract checks; each project repo maps them to commands in
`.north/checks.yaml`:

```yaml
test: uv run pytest
lint: uv run ruff check .
```

Exit code is the only contract — output is captured (last 80 lines) for the
next session to read, never machine-parsed. Per-gate `policy` controls
missing checks: `required` (default) fails the gate, `if-present` skips.

Per-task results in Borealis include each session's handoff note, gate
reports, and a session manifest (session id, transcript path, tokens, cost,
duration). Full transcripts are exported to
`~/.north/aurora/transcripts/<project>/<task>/<session_id>.json`.

## Board objects (Borealis)

The board is a git repo; every mutation is exactly one board commit through
the REST API (or the MCP surface, which calls the same service layer).

### Conversations

Work intake. A condensed design conversation is shipped onto the board as
`projects/<name>/conversations/<id>.md` (frontmatter + body) and lands
`pending`. Status only moves forward (`pending → decomposing → decomposed`)
and conversations are never deleted via the API — they are audit objects.
`GET /api/conversations/pending` serves the decomposition queue, oldest
first. On completion, `decomposed_into` lists the created features/tasks and
a companion `<id>.result.md` holds the decompose session's handoff note.

### Decomposition

Aurora's supervisor serves two queues, decomposition first: while pending
conversations exist, no project task is taken. Each conversation is
decomposed by a session (profile `decompose`, board access through the
`/mcp/decomposer/` grant) running in a detached worktree on the project's
`base_branch` — it reads the repo and the board, then creates draft
features/tasks stamped `decomposed_from`. The harness owns the
deterministic shell:

- **Docs-only guard** — repo changes from the session are committed to
  `base_branch` only if every changed path is under `docs/` or `AGENTS.md`
  (`DOCS_ALLOWLIST`); any other diff is discarded wholesale and noted in
  the result. The decomposer distills durable decisions, never code.
- **Bookkeeping** — `decomposed_into` is computed from a before/after board
  diff (never from agent self-report) and written to the conversation with
  the session's handoff note + manifest as the conversation result.
- **Failure** — a failed session returns the conversation to `pending`
  with the failure noted; the supervisor backs the conversation off for
  5 minutes so it doesn't hot-loop.

Managed clones and feature branches are created on demand (clone from the
project's `ssh_url`, branch from `base_branch`) the first time a task or
conversation needs them.

### Draft lifecycle

Created features and tasks always land `draft` (server-enforced — this is
the human gate before execution spend). The promote verb is the only way
out of draft: `POST .../features/{f}/promote` (draft → open),
`POST .../tasks/{id}/promote` (draft → ready; the feature must be promoted
first). Status PATCHes are gated by a server-side transition table — illegal
jumps (e.g. `draft → done`) are rejected with 409.

### Comment threads

Append-only companion files: `<task>.thread.md` and `_feature.thread.md`.
Entries are typed `[question] / [answer] / [note]` with author + timestamp
(`GET/POST .../comments`); there are no edit or delete endpoints. A task
blocked with `blocked_reason: question` flips back to `ready` when an
`[answer]` comment is posted — thread append and status flip land as one
board commit. `blocked_reason` distinguishes `question` (waiting on a
human), `dependency`, and `infra` (auth/config failures stamped by Aurora).

### Split

`POST .../tasks/{id}/split` replaces an oversized task with children in one
atomic board commit: children inherit the parent's `depends_on` and carry
`split_from`; dependents of the parent are re-pointed to all children; the
parent becomes `superseded` (kept for audit). Tasks that are
`done`/`in_progress`/`superseded` cannot be split.

### MCP surface

Borealis also exposes the board over MCP (streamable HTTP), mounted in the
same process beside REST — REST stays canonical; MCP is a surface. One
endpoint per grant set, each exposing only its tools:

| Grant         | Mount              | Tools beyond reads                                  |
|---------------|--------------------|-----------------------------------------------------|
| `decomposer`  | `/mcp/decomposer`  | `create_feature`, `create_task`                     |
| `implementer` | `/mcp/implementer` | `add_comment`, `split_task`                         |
| `reviewer`    | `/mcp/reviewer`    | `add_comment`, `promote_draft`, `create_conversation` |
| `cockpit`     | `/mcp/cockpit`     | `add_comment`, `promote_draft`, `create_conversation` |

All grants share the read tools (queue, features, tasks, review list,
conversations, comments). Optional bearer tokens per grant via
`MCP_TOKENS=grant:token,...` (defense-in-depth; the service binds loopback
only and must never be exposed publicly). Configure clients with the
trailing slash (`http://127.0.0.1:8001/mcp/cockpit/`) — the bare path
answers with a 307 redirect that not every MCP client follows.

### Reading feature history

Features merge with `--no-ff`, so `git log --first-parent main` reads as one
line per feature, while stage-granular commits stay reachable for
`blame`/`bisect` (use `git bisect --first-parent` for feature granularity).

---

## Human loop

### Notifications

Human-gate events (conversation shipped/decomposed, task blocked on a
question, task failed, feature ready for review) flow through a notifier in
Borealis; Aurora carries a mirror for its own service-health warnings.
Transports are config:

| Env var | Default | Meaning |
|---------|---------|---------|
| `NOTIFY_TRANSPORT` | `log` | `log` or `telegram` (degrades to log when unconfigured) |
| `TELEGRAM_BOT_TOKEN` / `TELEGRAM_CHAT_ID` | empty | Telegram bot credentials (outbound HTTPS only; nothing inbound) |
| `NOTIFY_DEDUPE_WINDOW_S` | `300` | identical (kind, fields) events collapse to one send |
| `NOTIFY_RATE_LIMIT_PER_MIN` | `20` | global cap — a retry loop can never storm the phone |
| `LOG_NOTIFY_DEDUPE_WINDOW_S` | `3600` | dedupe window for WARNING+ log forwarding (per logger + message template) |

Sending is queued to a background thread and never blocks or fails a board
mutation. Service health has two layers: WARNING+ log records forward
through the notifier (deduped), and `OnFailure=north-notify-failure@%n` on
every North unit fires `scripts/notify-failure.sh` (curl, works even when
the process can't self-report). ntfy (self-hosted) is the recorded swap
candidate — the transport interface makes that a config change.

### Review briefs

When a feature enters review, Aurora assembles a deterministic brief —
task list + statuses, accumulated handoff notes, final gate reports, and a
`git diff --stat` against the base branch — and posts it on the feature
thread as a `[note]` by `aurora/brief`. That note landing is what emits the
`feature_review` notification, carrying the brief's first line
(`feat-x ready: 4 tasks, +812/-147, gates green`).

### Refine rule

Creating a task on a feature in `review` reverts the feature to
`in_progress` in the same board commit; it returns to review when all
tasks complete again. "Accept-but" is one cockpit motion: add refinement
tasks, promote them, done.

### The cockpit

`scripts/cockpit.sh` opens (or attaches to) `north-design` — a persistent
tmux session running Claude Code in the `cockpit/` workspace. Desk and
phone attach to the same session over SSH, so a conversation started on a
walk continues at the desk. The workspace pins:

- `.mcp.json` — Borealis MCP with the **cockpit** grant (board reads +
  `create_conversation`, `add_comment`, `promote_draft`).
- `.claude/settings.json` — deny Edit/Write/mutating Bash; allow read-only
  repo inspection; managed clones (`~/.north/aurora/repos`) readable for
  review walks.
- `CLAUDE.md` — the cockpit role: condense + ship conversations (after
  human approval), curate/promote drafts, answer blocked questions,
  review briefings.

Safety asymmetry, by design: the cockpit understands and curates but holds
no verdict verbs. **Approve / rollback / reject are CLI-only**
(`north feature approve <project> <feature>` over SSH) — a recommendation in the cockpit is
never an execution.

### Voice (planned)

Driving the cockpit by voice is a planned future feature, not yet
implemented. An earlier attempt (see `docs/plans/026`, deferred) is on hold
until after the M1–M7 migration completes.

---

## North CLI

A single `north` binary drives both services. Operators install it with
`uv tool install north` (or `pipx install north`); for development run
`scripts/install-dev.sh` to symlink the editable console script onto your PATH
(see [Development](#development)).

```
north <command> [options]
```

| Command                                                | Description                            |
|--------------------------------------------------------|----------------------------------------|
| `status`                                               | Combined Aurora + Borealis status      |
| `status aurora` / `status borealis`                    | Single-service status                  |
| `logs [--project <name>]`                              | Stream agent output events             |
| `pause` / `resume`                                     | Pause/resume the task runner           |
| `service aurora <start\|stop\|restart\|enable\|disable\|status>`   | Manage the aurora systemd unit |
| `service borealis <start\|stop\|restart\|enable\|disable\|status>` | Manage the borealis systemd unit |
| `service status`                                       | Aggregate status of both units         |
| `projects list`                                        | List registered projects               |
| `projects show <project>`                              | Show a project's details               |
| `projects register <ssh_url> [--name] [--base-branch] [--auto-merge]` | Register a project      |
| `projects update <project> [--base-branch] [--auto-merge \| --no-auto-merge]` | Update a project's settings |
| `projects unregister <project> [-y]`                   | Unregister a project                   |
| `feature create <project> <title> [--description] [--depends-on ...]` | Create a feature        |
| `feature show <project> <feature>`                     | Show a feature                         |
| `feature status <project> <feature> <status>`          | Set a feature's status                 |
| `feature delete <project> <feature> [-y]`              | Delete a feature (draft tasks only)    |
| `feature requeue <project> <feature>`                  | Re-open a feature, reset tasks to ready|
| `feature promote <project> <feature>`                  | Promote a draft feature to open        |
| `feature approve <project> <feature>`                  | Approve and merge a feature branch     |
| `feature rollback <project> <feature>`                 | Roll back a merged feature             |
| `feature reject <project> <feature> [-y]`              | Reject and discard a feature branch    |
| `feature list [--project <name>] [--archived] [--review]` | List features (filter/archived/review) |
| `task create <project> <feature> <title> --pipeline <name> [--body \| --body-file] [--depends-on ...]` | Create a task |
| `task show <project> <feature> <task_id>`              | Show a task (including result)         |
| `task list <project> <feature> [--status <status>]`    | List a feature's tasks                 |
| `task status <project> <feature> <task_id> <status>`   | Set a task's status                    |
| `task delete <project> <feature> <task_id> [-y]`       | Delete a task                          |
| `task promote <project> <feature> <task_id>`           | Promote a draft task to ready          |
| `task split <project> <feature> <task_id> --tasks-json <json> \| --tasks-file <path>` | Split a task into children |
| `conversation create <project> <title> [--content \| --content-file] [--source text\|voice]` | Ship a conversation |
| `conversation list <project>`                          | List a project's conversations         |
| `conversation show <project> <conversation_id>`        | Show a conversation (with result)      |
| `conversation status <project> <conversation_id> <status>` | Set a conversation's status        |
| `comment add <project> <feature> [--task-id <id>] [--kind] [--author] <text>` | Comment on a task/feature |
| `comment list <project> <feature> [--task-id <id>]`    | Print a thread                         |

---

## Development

```bash
# Install deps
uv sync

# Put `north` on your PATH (symlinks the editable workspace console script)
scripts/install-dev.sh

# Run tests
uv run --with pytest,pytest-asyncio,httpx python -m pytest aurora/tests/ borealis/tests/ north/tests/ --ignore=aurora/tests/integration

# Lint
uv run ruff check .

# Run a service locally (without systemd)
uv run --package aurora uvicorn aurora.service.main:app --port 8000
uv run --package borealis uvicorn borealis.service.main:app --port 8001
```

---

## Repository layout

```
north/
  aurora/           # Agentic execution engine
    aurora/
      service/      # FastAPI app, stage runner, session runner, git operations
    tests/
  borealis/         # Board state manager
    borealis/
      service/      # FastAPI app, board parser, task resolver
    tests/
  north/            # Unified `north` CLI (drives both services)
    north/
      cli/          # parser, lazy NorthContext, command modules, clients
    tests/
  scripts/
    install.sh      # One-shot install script (services)
    install-dev.sh  # Symlink the `north` CLI onto PATH for development
  systemd/
    aurora.service
    borealis.service
    opencode.service
  docs/
    design/         # Design specs and planned features
  pyproject.toml    # uv workspace root
```
