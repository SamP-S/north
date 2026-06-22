## 16. CLI

The `aurora` CLI is a thin HTTP client that talks to `aurora.service` on `127.0.0.1`. It is the primary operator interface for monitoring and control. Board edits are made directly on the filesystem; the CLI does not expose board CRUD.

### 16.1 Commands

**Observation**

| Command | Description |
|---|---|
| `aurora status` | Current runner state (`running\|paused\|idle`), active task and project, OAuth health, monthly credit estimate |
| `aurora features [--project <project>]` | All active features across all projects; `--project` filters to a single project |
| `aurora queue [--project <project>]` | Pending and active tasks ordered by dependency depth then `ready_at`; `--project` filters to a single project |
| `aurora logs [--project <project>]` | Stream live agent output via SSE (`/api/events`); shows `agent.output` events only, prefixed with `[{project}/{feature}/{task_id}]`; `--project` filters to a single project; exits on `Ctrl+C` |

**Runtime control** — requires service to be running

| Command | Description |
|---|---|
| `aurora pause` | Pause at next safe boundary (see §9.3 in [backend API](09_backend-api.md)); discards uncommitted agent work |
| `aurora resume` | Resume from the recorded pipeline step with all prior artifacts as context |

**Process lifecycle** — wraps systemd

| Command | Description |
|---|---|
| `aurora start` | `systemctl start aurora` |
| `aurora stop` | `systemctl stop aurora` |

**Feature review** — for features in `status: review`

| Command | Description |
|---|---|
| `aurora review [--project <project>]` | Features in `status: review` needing operator action; `--project` filters to a single project |
| `aurora approve <project/feature>` | Merge feature branch into `base_branch` locally and push; archive board and remove worktree on success; report conflicts and abort if merge fails |
| `aurora rollback <project/feature>` | Reset feature branch to `base_branch` HEAD; reset all tasks to `ready`; feature → `open`; runner picks it up again |
| `aurora reject <project/feature>` | Reset feature branch to `base_branch` HEAD; feature → `closed`; archive board and remove worktree |

**Project management**

| Command | Description |
|---|---|
| `aurora projects` | List all registered projects |
| `aurora register <ssh_url> [--name <name>]` | Register a new project — project name defaults to repo name from SSH URL; clones repo, creates board dirs, updates `projects.yaml`, commits `[board:project]` |
| `aurora unregister <project>` | Unregister a project — always proceeds; removes board, managed clone, all worktrees, and `projects.yaml` entry; commits `[board:project]`; prints a warning listing any active or in-progress features before executing so the operator knows what work is being discarded |

### 16.2 Implementation notes

- Implemented in `cli/` as a standalone Python entry point; installed alongside the service via `uv`
- All observation and control commands call the REST API; `aurora logs` opens an SSE connection; `aurora projects`, `aurora features`, `aurora review`, and `aurora queue` pass optional `--project` as a query parameter
- `aurora start` / `aurora stop` invoke `systemctl --user` directly via subprocess
- `aurora approve` / `aurora rollback` / `aurora reject` call the feature review API endpoints (see §9.2 in [backend API](09_backend-api.md))
- `aurora register` / `aurora unregister` call the project management API endpoints (see §9.2 in [backend API](09_backend-api.md))
- Output is plain text; no interactive TUI for v1
