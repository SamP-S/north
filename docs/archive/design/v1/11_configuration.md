## 11. Configuration & secrets

### 11.1 `.env`

Per-agent values override these defaults. Per-step retry limits are defined in the pipeline file (see [pipelines](15_pipelines.md)). Git host SSH key is a standard `~/.ssh/` setup.

**Paths**

| Variable | Default | Notes |
|---|---|---|
| `AURORA_HOME` | `~/.aurora` | Root directory for all runtime data — repos, worktrees, and data; can be pointed at a different disk or partition |
| `BOARD_REPO_SSH_URL` | — | SSH URL of the board repo (e.g. `git@github.com:user/borealis.git`); cloned to `aurora/board/` on install |

**Auth & cloud usage**

| Variable | Default | Notes |
|---|---|---|
| `MONTHLY_SDK_CREDIT_USD` | `20` | Account usage-credits must be **disabled** |
| `MONTHLY_SOFT_CAP_USD` | `18` | Pause threshold with headroom under the credit limit |
| `BILLING_CYCLE_DAY` | `1` | Day of month the Anthropic subscription renews; spend counter resets on this day each month |

> **`ANTHROPIC_API_KEY` must NOT be set.** The Agent SDK will prefer an API key over the OAuth credential store, bypassing the subscription credit entirely and billing against the API instead.

**Limits & tuning**

| Variable | Default |
|---|---|
| `CLAUDE_CODE_MAX_OUTPUT_TOKENS` | `8000` |
| `MAX_TURNS_PER_CALL` | `20` |
| `MAX_BUDGET_USD_PER_CALL` | `0.50` |
| `AGENT_TIMEOUT_S` | `900` |
| `RUNNER_CONCURRENCY` | `1` |
| `COOLDOWN_SECONDS` | `300` |
| `POLL_INTERVAL_S` | `5` |

**Local execution**

| Variable | Default | Notes |
|---|---|---|
| `OLLAMA_BASE_URL` | `http://127.0.0.1:11434` | Ollama native `/api/chat` endpoint base; must be reachable at service start |
| `OLLAMA_DEFAULT_NUM_CTX` | `4096` | Default context window (tokens) for local agents that do not declare `num_ctx` |

**Notifications**

| Variable | Notes |
|---|---|
| `TELEGRAM_BOT_TOKEN` | |
| `TELEGRAM_CHAT_ID` | |

### 11.2 `projects.yaml` (registry only)

Top-level `schema_version` integer. Each project entry has: `ssh_url`, `base_branch`. The project name (key) defaults to the repo name derived from the SSH URL but can be overridden via `aurora register --name`. Aurora derives the local clone path as `$AURORA_HOME/repos/{project}`. Agent and pipeline overrides live in the board repo at `board/projects/{project}/agents/` and `board/projects/{project}/pipelines/`.

Example:

```yaml
schema_version: 1
projects:
  my-project:
    ssh_url: git@github.com:owner/my-project.git
    base_branch: main
```
