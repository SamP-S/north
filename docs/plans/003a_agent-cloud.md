# 003a — Agent Execution: Cloud (Claude Agent SDK)

## Summary

Implement cloud agent execution via the Claude Agent SDK: agent definition loading and merging, context assembly, `query()` invocation with all guards, artifact parsing, and error handling (`RateLimitEvent`, OAuth failure, budget/turn/timeout). Depends on 001 (board models) and the artifact schema from 004.

## Files to Create / Modify

```
service/
  execution/
    __init__.py
    agent_prepare.py           # load + merge agent definitions; resolve context paths
    cloud.py                   # query() invocation + ResultMessage → artifact
    artifacts.py               # artifact frontmatter parser (shared with local executor)
definitions/
  agents/
    basic_mapper.md            # example global agent definition
    basic_coder.md
    reviewer.md
    qa_checker.md
tests/
  test_agent_prepare.py
  test_artifacts.py
  test_cloud.py
```

## Todo

- [x] 1. `service/execution/artifacts.py` — parse artifact Markdown: extract YAML frontmatter block; validate required fields (`agent`, `confidence`, `status`, `summary`); validate `confidence` ∈ `{high, medium, low, blocked}`; validate `status` ∈ `{complete, failed, blocked}`; raise `ArtifactParseError` with message on any failure (used by retry logic)
- [x] 2. `service/execution/agent_prepare.py` — `load_agent_definitions(aurora_path, board_path, project)`: read all `.md` files from `definitions/agents/`; overlay project-specific agents from `board/projects/{project}/agents/` by `name`; parse frontmatter fields from §6.2; return `dict[str, AgentDefinition]`
- [x] 3. `service/execution/agent_prepare.py` — `prepare_agent(agent_name, agent_defs, worktree_path)`: resolve agent by name (raise `AgentPrepareError` → `status: blocked` if missing or invalid YAML); resolve each `context` path relative to worktree root; log warning + skip missing paths; return resolved `AgentDefinition` with context file contents loaded
- [x] 4. `service/execution/cloud.py` — `run_cloud_step(agent_def, artifacts, worktree_path, task_state)`: build system prompt (preset + role prompt + context file blocks with `--- context: {path} ---` delimiters); call `query()` with model, working dir, tools, `disallowedTools` merged with `GLOBAL_BASH_DENYLIST`, `permissionMode`, `maxTurns`, `max_budget_usd`, `setting_sources=["project"]`, all prior artifacts as user context; apply `CLAUDE_CODE_MAX_OUTPUT_TOKENS` via env; apply `AGENT_TIMEOUT_S` via `asyncio.wait_for`
- [x] 5. `service/execution/cloud.py` — handle `ResultMessage`: extract text output; parse as artifact via `artifacts.py`; on `ArtifactParseError` return parse error + attempt count; accumulate token/cost telemetry into `TaskState`
- [x] 6. `service/execution/cloud.py` — handle `RateLimitEvent`: set `queue_paused = True`; fire Telegram "rate-limit / credit exhausted"; return `status: queued` to re-enqueue
- [x] 7. `service/execution/cloud.py` — handle `authentication_failed`: fire Telegram "OAuth authentication failed — re-run `claude auth login`"; return `status: blocked`
- [x] 8. `service/execution/cloud.py` — handle timeout (`asyncio.TimeoutError`): kill subprocess; reset worktree; count as attempt; return `status: failed` if attempts exhausted
- [x] 9. Example agent definitions in `definitions/agents/` — `basic_mapper.md`, `basic_coder.md`, `reviewer.md`, `qa_checker.md`; each with valid frontmatter (name, description, model, tools, permission_mode, max_turns, max_budget_usd) and a role prompt body
- [x] 10. Unit tests — artifact parse: valid artifact; missing `confidence`; invalid `confidence` value; missing frontmatter block; agent merge: global only; project override replaces global; project adds new agent; context path resolution: exists → loaded; missing → warning + skipped
- [x] 11. Run `uv run ruff check .` and `uv run mypy service/` — fix all errors

## Change History

- [2026-06-07] All items complete. 36/36 tests pass, ruff clean, mypy clean (17 source files). SDK uses `ClaudeAgentOptions` (not `ClaudeCodeOptions`) — fallback import handles both. Private helpers `_build_system_prompt`, `_build_user_prompt`, `_merge_disallowed` extracted for testability.
