## 13. Testing strategy

### 13.1 Unit

- Frontmatter / task parsers
- Dependency resolver + queue ordering (cooldown, shallowest-first)
- Feature-dependency eligibility
- Commit-prefix filter
- Model resolution / provider inference
- Agent merge (global + project override)
- Context path resolution — project-root-relative paths resolved against worktree; missing paths warn and skip
- Pipeline loader — YAML parsing, required field validation (missing `confidence` levels, missing `on_fail`), step graph validation (missing ids, cycles, unreachable steps)
- Pipeline step execution contract — confidence routing, check pass/fail, `on_fail` routing, attempt counting
- `task_ingest` artifact production — correct frontmatter (`agent: system, confidence: high, status: complete`), task id + title + body as content
- Artifact frontmatter parser
- Built-in check runners (`qa`, `tests`, `not_empty`)
- Billing cycle reset — `BILLING_CYCLE_DAY` alignment, reset on correct day, no reset mid-cycle
- Feature lifecycle transitions — `open → in_progress → review → merged/closed`
- Migration scripts

### 13.2 Integration

- Board HEAD polling — new commit detected, loop wakes immediately
- Code commit to project branch + board commit to board repo as separate operations
- Full artifact chain — `task_ingest` artifact `[0]` passed through pipeline steps; each step receives all prior artifacts
- Feature → `review` transition when all tasks reach `done`
- `aurora approve` — clean merge into `base_branch`, push, board archive, worktree removal; conflict returns `409` and leaves feature in `review`
- `aurora rollback` — branch reset to `base_branch` HEAD, all tasks → `ready`, feature → `open`
- `aurora reject` — branch reset, board archive, worktree removal, feature → `closed`
- Archive-on-merge in the board repo
- Pipeline execution — full step sequence with artifact passing, check failure retry loop, `blocked` early exit
- Pause at safe boundary — pause signal during `agent_prepare` halts before `run_pipeline`; pause during `update_board` completes the node first
- `RateLimitEvent` pause
- Soft-cap pause
- `max_budget_usd` / `max_turns` enforcement

### 13.3 Smoke

Manual, run against a demo project after significant changes (see [planned features](99_planned-features.md) for the full demo project plan).

**Install-time OAuth smoke test** — run as the final step of `install.sh` before printing access details:

- Make a minimal authenticated `query()` call (single-turn, no tools, trivial prompt)
- Assert the call returns without `authentication_failed` or a credential error
- Confirms that the SDK subprocess correctly inherits credentials from `~/.claude/.credentials.json` (or `CLAUDE_CODE_OAUTH_TOKEN` for headless installs)
- Fails fast at install time rather than silently at first task execution
