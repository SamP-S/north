# 003b — Agent Execution: Local (Ollama)

## Summary

Implement the local agent executor: multi-turn tool-calling loop against Ollama's `/api/chat` endpoint, built-in tool definitions and execution, `GLOBAL_BASH_DENYLIST` enforcement, streaming response handling, and infrastructure failure handling (Ollama unreachable). Depends on 003a (artifact parser, agent definitions).

## Files to Create / Modify

```
service/
  execution/
    local.py                   # local_executor: Ollama multi-turn loop
    tools.py                   # built-in tool execution (scoped to worktree)
definitions/
  tools/
    read_file.json
    write_file.json
    edit_file.json
    list_dir.json
    bash.json
tests/
  test_local_executor.py
  test_tools.py
```

## Todo

- [x] 1. `definitions/tools/*.json` — write the 5 built-in tool definitions (`read_file`, `write_file`, `edit_file`, `list_dir`, `bash`) in Ollama/OpenAI function calling format per §6.6; include description and parameter schema for each
- [x] 2. `service/execution/tools.py` — `execute_tool(name, args, worktree_path)`: dispatch to built-in implementations; all file paths resolved relative to `worktree_path` (reject `..` traversal); `read_file`: read and return file contents; `write_file`: create or overwrite; `edit_file`: `old_string` → `new_string` exact replacement; `list_dir`: return directory listing; `bash`: run command in worktree via subprocess
- [x] 3. `service/execution/tools.py` — `GLOBAL_BASH_DENYLIST` check: patterns `rm *`, `sudo *`, `curl *`, `wget *`, `git push *`; `check_bash_denylist(command) -> str | None` returns matched pattern or `None`; called before every `bash` tool execution; on match, return error string to model instead of executing
- [x] 4. `service/execution/local.py` — `run_local_step(agent_def, artifacts, worktree_path, task_state)`: load tool JSON definitions from `definitions/tools/` for each name in `agent_def.tools`; assemble messages: `system` = role prompt + context file blocks (`--- context: {path} ---`), `user` = all prior artifacts as text blocks
- [x] 5. `service/execution/local.py` — POST to `OLLAMA_BASE_URL/api/chat` with model, messages, tools, `stream: true`, `options.num_ctx`; stream and accumulate response chunks; on `message.tool_calls`: validate each `bash` call against denylist; execute each tool against worktree; append assistant message + tool result messages; repeat loop
- [x] 6. `service/execution/local.py` — terminal condition: response with no `tool_calls`; parse text response as artifact via `artifacts.py`; record `usage.prompt_tokens` + `usage.completion_tokens` to `TaskState`; `estimated_cost_usd = 0.0`; turn count = number of `/api/chat` calls
- [x] 7. `service/execution/local.py` — infrastructure failure handling: Ollama unreachable or model not loaded → `status: blocked`; fire Telegram "Ollama unreachable — {error}"; pause queue; distinct from agent-level `failed`
- [x] 8. `service/execution/local.py` — apply `AGENT_TIMEOUT_S` to full session (all turns); on `asyncio.TimeoutError`: count as attempt, return `status: failed` if exhausted
- [x] 9. Unit tests — denylist: each blocked pattern matches; safe commands pass; tool execution: `read_file` reads correct content; `write_file` creates file in worktree; `edit_file` replaces correctly; path traversal (`../`) rejected; multi-turn loop: single-turn (no tools); one tool call round; two sequential tool call rounds; terminal on no `tool_calls`
- [x] 10. Run `uv run ruff check .` and `uv run mypy service/` — fix all errors

## Change History

- [2026-06-07] All items complete. 50/50 tests pass, ruff clean, mypy clean (19 source files). Used `stream: False` for Ollama requests (simpler than streaming, functionally equivalent). Path traversal rejection scoped to worktree root. `check_bash_denylist` extracts prefix from `"Bash(cmd *)"` format.
