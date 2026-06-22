## 14. Suggested build sequencing

Prove the board structure and queue before any LLM; bring up agentic execution on the reliable cloud path before fighting local-model tool-calling.

1. **Board core (no agents)**
   - Parsers and board repo layout (`projects/{project}/board/`)
   - Prefixed commits to board repo + push
   - FastAPI skeleton (`/health`, `/status`, `/queue`, `/events`, `/control`)
   - Goal: CLI can list tasks and see runner state

2. **Queue + dependency resolver**
   - Task aggregation across all active features; feature deps vs main
   - Supervisor loop with a stub executor
   - Cooldown, shallowest-first ordering

3. **Agent execution (cloud-first)**
   - `claude auth login` + OAuth
   - `agent_prepare` (merge global agents from aurora + project agents from board repo)
   - Single-step pipeline execution via `claude-agent-sdk` with budget/turn guards + `RateLimitEvent`/OAuth handling
   - Artifact production and parsing
   - Code commit to project branch + board status commit to board repo
   - Wire local model routing (Ollama `local_executor`)

4. **Pipeline engine**
   - Pipeline YAML loader and graph validator
   - Full step sequencing with artifact passing
   - Confidence routing (`high`/`medium`/`low`/`blocked`)
   - Built-in checks (`qa`, `not_empty`) with retry loop
   - `on_fail` routing and `max_attempts` enforcement
   - Credit/rate-limit controls

5. **Git integration**
   - Committed `_feature.md` detection — branch + worktree creation, detect-and-adopt existing branch
   - Invalid frontmatter detection + Telegram notification
   - Post-commit hook placeholder

6. **Feature review flow**
   - Runner sets `status: review` when all tasks → `done`; Telegram notification
   - `aurora approve` — local merge, push, board archive, worktree removal, Telegram
   - `aurora rollback` — branch reset, tasks → `ready`, feature → `open`, Telegram
   - `aurora reject` — branch reset, board archive, worktree removal, Telegram
   - Conflict handling on approve — abort, report to operator, feature stays `review`

7. **CLI and control**
   - `aurora status`, `aurora queue`, `aurora logs`
   - `start`/`stop` (systemd), `pause`/`resume` with safe-boundary halt and worktree rollback
   - `aurora register [--name]`, `aurora unregister`

8. **Hardening**
   - Restart/crash handling (stuck `in_progress` → `queued` on startup)
   - Board frontmatter migrations
   - Smoke tests
