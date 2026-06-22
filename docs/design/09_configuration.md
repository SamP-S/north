## 9. Configuration & secrets

### 9.1 `.env`

Loaded from `.env` (working dir) and `~/.north/.env`. Git host SSH key is a
standard `~/.ssh/` setup.

**Paths & service**

| Variable | Default | Notes |
|---|---|---|
| `BOARD_REPO_SSH_URL` | — | SSH URL of the board repo; cloned to the board path on install |
| `NORTH_HOME` | `~/.north` | Root directory for runtime data |
| `BOARD_PATH` | `~/.north/board` | Local board repo clone |
| `NORTH_PORT` | `8001` | Loopback bind port (REST + MCP) |
| `POLL_INTERVAL_S` | `5` | Board `HEAD` poll interval |
| `COOLDOWN_SECONDS` | `300` | Delay before a `ready` task becomes `queued` |

**MCP**

| Variable | Default | Notes |
|---|---|---|
| `MCP_TOKENS` | empty | Per-grant bearer tokens, `grant:token,grant:token` (empty = no token required) |

**Notifications**

| Variable | Default | Notes |
|---|---|---|
| `NOTIFY_TRANSPORT` | `log` | `log` or `telegram` |
| `TELEGRAM_BOT_TOKEN` / `TELEGRAM_CHAT_ID` | empty | Telegram credentials (outbound only) |
| `NOTIFY_DEDUPE_WINDOW_S` | `300` | Identical `(kind, fields)` events collapse to one send |
| `NOTIFY_RATE_LIMIT_PER_MIN` | `20` | Global send cap |
| `LOG_NOTIFY_DEDUPE_WINDOW_S` | `3600` | Dedupe window for WARNING+ log forwarding |

### 9.2 `projects.yaml` (registry only)

Top-level `schema_version` integer. Each project entry has `ssh_url`,
`base_branch`, and `auto_merge`. The project name (key) defaults to the repo name
derived from the SSH URL but can be overridden via `north projects register
--name`.

```yaml
schema_version: 1
projects:
  my-project:
    ssh_url: git@github.com:owner/my-project.git
    base_branch: main
    auto_merge: false
```
