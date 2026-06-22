## 7. CLI

The `north` CLI is a thin HTTP client that talks to `north.service` on
`127.0.0.1`. It is the primary operator interface for inspecting and editing the
board and managing the service. It is installed as a local tool
(`uv tool install north` or `pipx install north`); for development,
`scripts/install-dev.sh` symlinks the editable console script onto `PATH`.

### 7.1 Commands

**Observation**

| Command | Description |
|---|---|
| `north status` | Board service status (runner state, board-loaded flag) |
| `north queue [--project <name>]` | Pending and active tasks ordered by dependency depth then `ready_at` |

**Service lifecycle** — wraps `systemctl --user`

| Command | Description |
|---|---|
| `north service <start\|stop\|restart\|enable\|disable\|status>` | Manage the `north` systemd unit |

**Projects**

| Command | Description |
|---|---|
| `north projects list` | List registered projects |
| `north projects show <project>` | Show a project |
| `north projects register <ssh_url> [--name] [--base-branch] [--auto-merge]` | Register a project |
| `north projects update <project> [--base-branch] [--auto-merge \| --no-auto-merge]` | Update settings |
| `north projects unregister <project> [-y]` | Unregister a project |

**Features**

| Command | Description |
|---|---|
| `north feature create <project> <title> [--description] [--depends-on ...]` | Create a feature |
| `north feature show <project> <feature>` | Show a feature |
| `north feature edit <project> <feature> [--title] [--description] [--status] [--depends-on ...]` | Edit fields |
| `north feature status <project> <feature> <status>` | Set status |
| `north feature delete <project> <feature> [-y]` | Delete (draft tasks only) |
| `north feature requeue <project> <feature>` | Re-open, reset tasks to ready |
| `north feature promote <project> <feature>` | Promote a draft feature to open |
| `north feature list [--project <name>] [--archived] [--review]` | List features |

**Tasks**

| Command | Description |
|---|---|
| `north task create <project> <feature> <title> --pipeline <name> [--body \| --body-file] [--depends-on ...]` | Create a task |
| `north task show <project> <feature> <task_id>` | Show a task (with result) |
| `north task list <project> <feature> [--status <status>]` | List tasks |
| `north task edit <project> <feature> <task_id> [...]` | Edit fields |
| `north task status <project> <feature> <task_id> <status>` | Set status |
| `north task delete <project> <feature> <task_id> [-y]` | Delete a task |
| `north task promote <project> <feature> <task_id>` | Promote a draft task to ready |
| `north task split <project> <feature> <task_id> --tasks-json <json> \| --tasks-file <path>` | Split into children |

**Conversations and comments**

| Command | Description |
|---|---|
| `north conversation create <project> <title> [--content \| --content-file] [--source text\|voice]` | Ship a conversation |
| `north conversation list <project>` | List conversations |
| `north conversation show <project> <conversation_id>` | Show a conversation (with result) |
| `north conversation status <project> <conversation_id> <status>` | Set status |
| `north comment add <project> <feature> [--task-id <id>] [--kind] [--author] <text>` | Comment on a task/feature |
| `north comment list <project> <feature> [--task-id <id>]` | Print a thread |

### 7.2 Implementation notes

- Implemented in `north/cli/` as a standalone Python entry point; installed
  alongside the service via `uv`.
- All board commands call the REST API through a lazily-constructed board client
  (`NorthContext.board`); a command that never talks to the service does not
  depend on it being reachable.
- `north service …` invokes `systemctl --user` directly via subprocess.
- Output is plain text; no interactive TUI.
