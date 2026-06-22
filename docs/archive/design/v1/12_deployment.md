## 12. Deployment & operations

### 12.1 systemd units

All services run as **user systemd units** (`systemctl --user`) under the operator's account, with a shared `.env`. `loginctl enable-linger <username>` must be run once at install time so units persist after the operator logs out. Logs accessible via `journalctl --user -u <service>`.

| Service | Type | Notes |
|---|---|---|
| `aurora.service` | persistent | FastAPI + async task runner; binds `127.0.0.1`; restart on failure; runs startup validation (§12.4) then resets stuck worktrees (see §5.7 in [orchestrator](05_orchestrator.md)); requires `claude auth login` |
| `ollama.service` | persistent | Start on boot; CUDA offload to RTX 3060 |

### 12.2 Ollama

`ollama pull mistral:7b-q4`, `ollama pull codellama:7b-q4`; CUDA offload to the RTX 3060; one 7B model resident at a time on 6 GB.

### 12.3 Install (`scripts/install.sh`)

Idempotent. Steps:

1. Pin Python 3.12; `uv sync` to create virtualenv and install from `pyproject.toml` (includes `claude-agent-sdk`, `ruff`, `mypy`, `pytest`, `pydeps`)
2. Install the Claude Code CLI
3. Run `claude auth login` once (OAuth) and claim the Agent SDK credit; confirm account **usage-credits disabled**; ensure `ANTHROPIC_API_KEY` is not set. For headless/non-interactive installs, use `claude setup-token` instead — this generates a long-lived `CLAUDE_CODE_OAUTH_TOKEN` that does not require a browser. Run a minimal SDK smoke test (`python -c "import asyncio; from claude_agent_sdk import query; ..."`) to confirm credentials are valid before continuing.
4. Create `$AURORA_HOME/{repos,worktrees,data}/` if they do not exist (default `~/.aurora`)
5. Clone the board repo (`BOARD_REPO_SSH_URL`) to `aurora/board/` — if the clone fails (remote not found, auth error, etc.) the install aborts with a clear error; the operator must create the remote board repo before running the install
6. Run `loginctl enable-linger $(whoami)` so user units persist after logout
7. Render and `systemctl --user enable --now` all units
8. Verify Ollama is reachable and required models are present (`ollama list`)
9. Print access details

### 12.4 Service startup validation

On every start, `aurora.service` runs the following checks before entering the supervisor loop:

1. **Board repo exists** — `aurora/board/` must be present and a valid Git repo; if not, the service fails to start with a clear log message directing the operator to run `install.sh`
2. **`projects.yaml` exists** — `aurora/board/projects.yaml` must be present; if missing, the service creates it with an empty projects registry and commits `[board:project]` to the board repo before continuing:

```yaml
schema_version: 1
projects: {}
```

Any `in_progress` tasks are reset to `queued` after these checks pass (see §5.7 in [orchestrator](05_orchestrator.md)).

### 12.5 Backup & DR

- **aurora repo** → its own private remote (engine code + global agent/pipeline definitions). Restore by cloning aurora and running `install.sh`.
- **Board repo** (`aurora/board/`) → its own private remote (all board state, registry, project-specific overrides). Pushed on every board commit. Restore by re-cloning via `BOARD_REPO_SSH_URL`.
- **Each project repo** → its own remote; feature branches pushed for code backup. Project repos contain only code + `CLAUDE.md` + `docs/ctx/`.
- `$AURORA_HOME/repos/`, `$AURORA_HOME/worktrees/`, and `$AURORA_HOME/data/` are all reconstructable; no backup needed.

### 12.6 Schema migrations

If the board frontmatter schema changes, a migration script is added to `scripts/` and run manually against all affected task/feature files.
