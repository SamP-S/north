# North

North is a background service that manages a **git-backed task board** over a
REST API (with a parallel MCP surface), plus a companion `north` CLI. It owns the
board state — projects, features, tasks, conversations, and comment threads — and
enforces their lifecycle. North does not execute tasks itself; an external agent
runtime (if any) drives work by talking to North over HTTP/MCP.

It runs as a systemd user service and is managed via the `north` CLI.

---

## Requirements

- Python 3.12+
- [uv](https://docs.astral.sh/uv/)
- A git repo to use as the task board (SSH access required)

---

## Install

**1. Set required environment variables**

Create `~/.north/.env`:

```env
BOARD_REPO_SSH_URL=git@github.com:your-org/your-board-repo.git
```

Optional overrides:

```env
NORTH_PORT=8001
NORTH_HOME=~/.north
BOARD_PATH=~/.north/board
```

**2. Run the install script**

```bash
bash scripts/install.sh
```

This will:
- Install Python dependencies via `uv sync`
- Install the `north` CLI as a local tool (`uv tool install`)
- Clone the board repo into `~/.north/board/`
- Enable user linger
- Install and enable `north.service` (and the failure-notification template) as
  systemd user units

---

## Service

| Service | Default port | Logs                                       |
|---------|-------------|--------------------------------------------|
| North   | 8001        | `journalctl --user -u north.service -f`    |

Run it locally without systemd:

```bash
uv run uvicorn north.service.main:app --port 8001
```

---

## Board objects

The board is a git repo; every mutation is exactly one board commit through the
REST API (or the MCP surface, which calls the same service layer).

### Conversations

Work intake. A condensed design conversation is shipped onto the board as
`projects/<name>/conversations/<id>.md` (frontmatter + body) and lands
`pending`. Status only moves forward (`pending → decomposing → decomposed`) and
conversations are never deleted via the API — they are audit objects.
`GET /api/conversations/pending` serves the decomposition queue, oldest first. On
completion, `decomposed_into` lists the created features/tasks and a companion
`<id>.result.md` holds the decomposition result. An external agent drains this
queue; North only owns the queue and the resulting board state.

### Draft lifecycle

Created features and tasks always land `draft` (server-enforced — this is the
human gate before any execution spend). The promote verb is the only way out of
draft: `POST .../features/{f}/promote` (draft → open),
`POST .../tasks/{id}/promote` (draft → ready; the feature must be promoted
first). Status PATCHes are gated by a server-side transition table — illegal
jumps (e.g. `draft → done`) are rejected with 409.

### Comment threads

Append-only companion files: `<task>.thread.md` and `_feature.thread.md`.
Entries are typed `[question] / [answer] / [note]` with author + timestamp
(`GET/POST .../comments`); there are no edit or delete endpoints. A task blocked
with `blocked_reason: question` flips back to `ready` when an `[answer]` comment
is posted — thread append and status flip land as one board commit.
`blocked_reason` distinguishes `question` (waiting on a human), `dependency`, and
`infra` (auth/config failures stamped by an external runtime).

### Split

`POST .../tasks/{id}/split` replaces an oversized task with children in one
atomic board commit: children inherit the parent's `depends_on` and carry
`split_from`; dependents of the parent are re-pointed to all children; the parent
becomes `superseded` (kept for audit). Tasks that are
`done`/`in_progress`/`superseded` cannot be split.

### MCP surface

North also exposes the board over MCP (streamable HTTP), mounted in the same
process beside REST — REST stays canonical; MCP is a surface. One endpoint per
grant set, each exposing only its tools:

| Grant         | Mount              | Tools beyond reads                                  |
|---------------|--------------------|-----------------------------------------------------|
| `decomposer`  | `/mcp/decomposer`  | `create_feature`, `create_task`                     |
| `implementer` | `/mcp/implementer` | `add_comment`, `split_task`                         |
| `reviewer`    | `/mcp/reviewer`    | `add_comment`, `promote_draft`, `create_conversation` |
| `cockpit`     | `/mcp/cockpit`     | `add_comment`, `promote_draft`, `create_conversation` |

All grants share the read tools (queue, features, tasks, review list,
conversations, comments). Optional bearer tokens per grant via
`MCP_TOKENS=grant:token,...` (defense-in-depth; the service binds loopback only
and must never be exposed publicly). Configure clients with the trailing slash
(`http://127.0.0.1:8001/mcp/cockpit/`) — the bare path answers with a 307
redirect that not every MCP client follows.

### Review briefs

When a feature enters review, a `[note]` authored by `north/brief` landing on the
feature thread is what emits the `feature_review` notification, carrying the
brief's first line (`feat-x ready: 4 tasks, +812/-147, gates green`). North emits
the event on the note landing; the brief content itself is assembled by whatever
external runtime posts it.

### Refine rule

Creating a task on a feature in `review` reverts the feature to `in_progress` in
the same board commit; it returns to review when all tasks complete again.
"Accept-but" is one motion: add refinement tasks, promote them, done.

### Reading feature history

Features merge with `--no-ff`, so `git log --first-parent main` reads as one line
per feature, while stage-granular commits stay reachable for `blame`/`bisect`
(use `git bisect --first-parent` for feature granularity).

---

## Notifications

Human-gate events (conversation shipped/decomposed, task blocked on a question,
task failed, feature ready for review) flow through a notifier in North.
Transports are config:

| Env var | Default | Meaning |
|---------|---------|---------|
| `NOTIFY_TRANSPORT` | `log` | `log` or `telegram` (degrades to log when unconfigured) |
| `TELEGRAM_BOT_TOKEN` / `TELEGRAM_CHAT_ID` | empty | Telegram bot credentials (outbound HTTPS only; nothing inbound) |
| `NOTIFY_DEDUPE_WINDOW_S` | `300` | identical (kind, fields) events collapse to one send |
| `NOTIFY_RATE_LIMIT_PER_MIN` | `20` | global cap — a retry loop can never storm the phone |
| `LOG_NOTIFY_DEDUPE_WINDOW_S` | `3600` | dedupe window for WARNING+ log forwarding (per logger + message template) |

Sending is queued to a background thread and never blocks or fails a board
mutation. Service health is surfaced by forwarding WARNING+ log records through
the notifier (deduped). ntfy (self-hosted) is the recorded swap candidate — the
transport interface makes that a config change.

---

## North CLI

The `north` CLI drives the board service. Operators install it with
`uv tool install north` (or `pipx install north`); for development run
`scripts/install-dev.sh` to symlink the editable console script onto your PATH
(see [Development](#development)).

```
north <command> [options]
```

| Command                                                | Description                            |
|--------------------------------------------------------|----------------------------------------|
| `status`                                               | Board service status                   |
| `queue [--project <name>]`                             | List active and queued tasks           |
| `service <start\|stop\|restart\|enable\|disable\|status>` | Manage the north systemd unit       |
| `projects list`                                        | List registered projects               |
| `projects show <project>`                              | Show a project's details               |
| `projects register <ssh_url> [--name] [--base-branch] [--auto-merge]` | Register a project      |
| `projects update <project> [--base-branch] [--auto-merge \| --no-auto-merge]` | Update a project's settings |
| `projects unregister <project> [-y]`                   | Unregister a project                   |
| `feature create <project> <title> [--description] [--depends-on ...]` | Create a feature        |
| `feature show <project> <feature>`                     | Show a feature                         |
| `feature edit <project> <feature> [--title] [--description] [--status] [--depends-on ...]` | Edit a feature |
| `feature status <project> <feature> <status>`          | Set a feature's status                 |
| `feature delete <project> <feature> [-y]`              | Delete a feature (draft tasks only)    |
| `feature requeue <project> <feature>`                  | Re-open a feature, reset tasks to ready|
| `feature promote <project> <feature>`                  | Promote a draft feature to open        |
| `feature list [--project <name>] [--archived] [--review]` | List features (filter/archived/review) |
| `task create <project> <feature> <title> --pipeline <name> [--body \| --body-file] [--depends-on ...]` | Create a task |
| `task show <project> <feature> <task_id>`              | Show a task (including result)         |
| `task list <project> <feature> [--status <status>]`    | List a feature's tasks                 |
| `task edit <project> <feature> <task_id> [...]`        | Edit a task's fields                   |
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
# Install deps (with dev tools)
uv sync --all-extras

# Put `north` on your PATH (symlinks the editable console script)
scripts/install-dev.sh

# Run tests
uv run pytest

# Lint and type-check
uv run ruff check .
uv run mypy north

# Run the service locally (without systemd)
uv run uvicorn north.service.main:app --port 8001
```

---

## Repository layout

```
north/
  north/            # the `north` package
    service/        # FastAPI app, board parser, task resolver, MCP surface
    cli/            # parser, lazy NorthContext, command modules, board client
  tests/            # service + CLI tests
  scripts/
    install.sh      # One-shot install script (service + CLI)
    install-dev.sh  # Symlink the `north` CLI onto PATH for development
  systemd/
    north.service
  docs/
    design/         # Design specs and planned features
  pyproject.toml    # single-package project config
```
