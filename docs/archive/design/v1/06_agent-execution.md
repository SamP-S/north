## 6. Agent execution

### 6.1 Cloud invocation (Claude Agent SDK)

Drive cloud agents through the Python Agent SDK (`pip install claude-agent-sdk`), `query()` — one fresh session per pipeline step, stateless. The SDK spawns the Claude Code CLI as a subprocess and has no authentication parameters of its own — it inherits credentials from the CLI's credential store (`~/.claude/.credentials.json` on Linux, written by `claude auth login`). Cloud calls draw on the Pro $20/month Agent SDK credit. `ANTHROPIC_API_KEY` must not be set — it sits above OAuth in the CLI's credential precedence chain and would bypass the subscription credit entirely. For non-interactive or headless installs, `claude setup-token` generates a long-lived `CLAUDE_CODE_OAUTH_TOKEN` env var as an alternative to browser-based login.

Each `query()` call is configured with: the agent's resolved model; the feature worktree as working directory; a system prompt comprising the `claude_code` preset, the agent's role prompt, and any declared context files appended with `--- context: {path} ---` delimiters (matching the local executor format — see §6.5); `setting_sources=["project"]` to load `CLAUDE.md`; per-agent allowed/disallowed tools and permission mode; `max_turns` and `max_budget_usd`; and all prior pipeline artifacts as context (see §15.6 in [pipelines](15_pipelines.md)). No agent roster is passed — agents do not delegate to sub-agents; all multi-agent orchestration is handled at the pipeline/runner level. The prompt instructs the agent to format its response as a Markdown artifact with the required frontmatter fields.

On completion, `ResultMessage` yields total cost, token usage, per-model usage breakdown, and the agent's text output — parsed as a Markdown agent artifact (see §15.6 in [pipelines](15_pipelines.md)). If the agent's response is missing frontmatter, contains invalid YAML, is missing the `confidence` field, or has an invalid `confidence` value, the parse failure is treated as a step failure: the error is logged, counted as an attempt, and the parse error is injected as context into the retry prompt. If attempts are exhausted, `on_fail` routing applies. `RateLimitEvent` triggers a queue pause; `authentication_failed` triggers a re-login alert via Telegram.

Guardrails: per-agent `max_budget_usd`, `max_turns`, `CLAUDE_CODE_MAX_OUTPUT_TOKENS` (default 8000; tool default 32000 over-spends), `AGENT_TIMEOUT_S` (default 900; kill → `status=timeout`, reset worktree, count as attempt).

### 6.2 Agent definitions, merging, provisioning

Markdown files with extended frontmatter. Globals in `aurora/definitions/agents/`; project agents in the board repo at `projects/{project}/agents/` — override globals by `name` or add roles.

Each agent definition is a Markdown file with a YAML frontmatter header and a role prompt body. Frontmatter fields:

| Field | Provider | Notes |
|---|---|---|
| `name` | both | |
| `description` | both | |
| `model` | both | alias or full ID; determines provider via §5.6 |
| `tools` | both | cloud: Claude Code tool allow list (e.g. `Read`, `Bash(ruff *)`); local: names resolved to `definitions/tools/{name}.json` |
| `disallowed_tools` | cloud only | merged with `GLOBAL_BASH_DENYLIST` (§6.3) |
| `permission_mode` | cloud only | `default\|acceptEdits\|plan\|dontAsk\|bypassPermissions` |
| `max_turns` | both | |
| `max_output_tokens` | cloud only | applied via env |
| `max_budget_usd` | cloud only | |
| `effort` | cloud only | `low\|medium\|high\|xhigh\|max` — controls the extended thinking budget; higher values allow the model more internal reasoning steps before responding, at higher token cost; use `low`/`medium` for routine tasks, `high`+  for complex planning or architecture work |
| `context` | both | list of project-root-relative paths to load and inject into the session |
| `num_ctx` | local only | Ollama context window size in tokens; overrides `OLLAMA_DEFAULT_NUM_CTX` for this agent |

The body is the agent's role prompt.

**SDK mapping (cloud).** Native to `AgentDefinition`: `description`, `prompt` (body), `tools`, `disallowedTools`, `model`, `maxTurns`, `permissionMode`, `effort`. Orchestrator-applied per invocation: `max_output_tokens` (env), `max_budget_usd` (top-level, cloud). `context` and `num_ctx` are Aurora extensions.

`agent_prepare`: read global agents from `aurora/definitions/agents/`; read project-specific agents from the board repo at `projects/{project}/agents/`; overlay project over global by `name`; build the `AgentDefinition` for the current step's agent only; resolve each agent's declared `context` paths against the project worktree root — paths that do not exist are logged as a warning and skipped. For local agents, resolve each `tools` name to `definitions/tools/{name}.json` at `agent_prepare` time — a missing definition causes `agent_prepare` to fail with `status: blocked`.

**Provisioning is programmatic**, not `.claude/agents/` discovery. The async task runner is the orchestrator. No roster is passed to `query()` — sub-agent delegation is not used in v1.

### 6.3 Tool permissions and Bash safety

**Cloud agents:** Scoped allow rules per agent (`Bash(pytest *)`, `Bash(ruff *)`; no bare `Bash`); a global deny list (`GLOBAL_BASH_DENYLIST`: `Bash(rm *)`, `Bash(sudo *)`, `Bash(curl *)`, `Bash(wget *)`, `Bash(git push *)`) merged into every cloud agent and effective **even under `bypassPermissions`** (a hard floor). This list is hardcoded in the engine and is not operator-configurable — it is a security invariant, not a tuning point. If a task legitimately requires a blocked command, the correct approach is to scope the agent's allowed tools explicitly in its definition rather than weakening the global floor. Optional `PreToolUse`/`can_use_tool` backstop blocking Bash that touches paths outside the worktree.

**Local agents:** Tool access is controlled entirely by the `tools` list in the agent definition — only tools named there are included in the Ollama request. `GLOBAL_BASH_DENYLIST` applies to the `bash` tool: the runner validates each `bash` tool call's command string against the denylist before execution and returns an error result to the model if matched. `disallowed_tools` and `permission_mode` are not applicable to local agents.

### 6.4 Output capture

**Cloud:** Each pipeline step produces an artifact from the `ResultMessage`: the agent's text output forms the artifact body; total cost, input/output/cache token counts, per-model cost and context window usage, turn count, and duration are recorded to `TaskState` telemetry.

**Local:** The `local_executor` records token counts from Ollama's response (`usage.prompt_tokens`, `usage.completion_tokens`); `estimated_cost_usd` is always `0.0`; turn count is the number of `/api/chat` calls made in the session (each tool-call round is one turn). The final text response forms the artifact body.

All artifacts accumulated during the pipeline run are written to `{id}-{slug}.result.md` by `update_board` in order — the full chain is preserved as an audit trail of every step's output.

### 6.5 Local invocation (Ollama)

Local agents are invoked via Ollama's native `/api/chat` endpoint at `OLLAMA_BASE_URL`. The `local_executor` implements a multi-turn tool execution loop. The native `/api/chat` endpoint is used rather than `/v1/chat/completions` — `options.num_ctx` is silently ignored on the OpenAI-compatible endpoint.

**Request shape:**

```json
{
  "model": "<agent.model>",
  "messages": [...],
  "tools": [...],
  "stream": true,
  "options": {
    "num_ctx": "<agent.num_ctx or OLLAMA_DEFAULT_NUM_CTX>"
  }
}
```

**Execution loop:**

1. Load tool JSON definitions from `definitions/tools/` for each name in the agent's `tools` field (resolved at `agent_prepare` time)
2. Assemble messages:
   - `system`: agent role prompt, followed by each declared context file's contents delimited with `--- context: {path} ---` headers
   - `user`: all prior pipeline artifacts as text blocks
3. POST to `OLLAMA_BASE_URL/api/chat` with model, messages, tools, `stream: true`, and `options.num_ctx`
4. If the response `message.tool_calls` is non-empty:
   a. Validate each `bash` tool call against `GLOBAL_BASH_DENYLIST`; return an error result to the model if matched
   b. Execute each tool call against the project feature worktree (`$AURORA_HOME/worktrees/{project}/{feature}`)
   c. Append the assistant message and one `{"role": "tool", "tool_name": "<name>", "content": "<result>"}` message per call
   d. Repeat from step 3
5. When the response contains no `tool_calls`: parse the text response as a Markdown artifact and return

`AGENT_TIMEOUT_S` applies to the full session (all turns combined). If Ollama is unreachable or the model is not loaded, the task transitions to `status: blocked`, a Telegram notification fires with the error details, and the queue pauses — this is a distinct infrastructure failure, not agent-level `failed`.

### 6.6 Tool definitions (`definitions/tools/`)

Tool definitions live in `aurora/definitions/tools/` — one JSON file per tool, named `{tool_name}.json`. Each file follows Ollama's tool definition format (identical to the OpenAI function calling schema):

```json
{
  "type": "function",
  "function": {
    "name": "read_file",
    "description": "Read the contents of a file from the project worktree.",
    "parameters": {
      "type": "object",
      "properties": {
        "path": {
          "type": "string",
          "description": "Path relative to the worktree root."
        }
      },
      "required": ["path"]
    }
  }
}
```

The tool name (filename stem) is the reference used in local agent definition `tools` fields. The runner implements the execution function for each built-in tool; all executions are scoped to the project feature worktree.

Aurora ships the following built-in tool definitions and implementations:

| Tool | Description |
|---|---|
| `read_file` | Read a file from the worktree |
| `write_file` | Write (create or overwrite) a file in the worktree |
| `edit_file` | Replace `old_string` with `new_string` in a file |
| `list_dir` | List directory contents in the worktree |
| `bash` | Run a shell command in the worktree (subject to `GLOBAL_BASH_DENYLIST`) |

Project-specific tool overrides are not supported in v1. All tool executions are scoped to `$AURORA_HOME/worktrees/{project}/{feature}`.
