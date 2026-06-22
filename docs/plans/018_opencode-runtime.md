# 018 — Opencode Runtime

## Summary

Second step of the v2 architecture: adopt `opencode serve` as the agent
runtime behind the `AgentRuntime` adapter from plan 017, then retire the
legacy SDK/Ollama execution code.

Phased: a spike validates the assumptions before any integration code is
written; cutover and legacy deletion only happen after the integration runs a
real task end-to-end.

## Phase 0 — Spike (no North code changes)

Validate against a locally running `opencode serve`:

- [x] S1. Session creation against an explicit directory (a North worktree);
      confirm file edits land in that worktree only
- [~] S2. Drive one existing North agent definition (role prompt + context
      files as the message) through an Anthropic model; confirm the artifact
      frontmatter round-trips parseably
- [x] S3. Same through an Ollama model (`mistral:7b`); measure artifact
      compliance rate over a handful of runs
- [~] S4. Permissions: confirm opencode config can deny the equivalent of
      `GLOBAL_BASH_DENYLIST` (rm/sudo/curl/wget/git push) non-interactively;
      confirm a denied call fails the step rather than hanging on a
      permission request
- [x] S5. Failure modes: behaviour on rate limit, bad auth, and timeout —
      map each to an `Outcome`
- [x] S6. Decide sync message vs `prompt_async`+poll for v1; record findings
      and chosen opencode version pin here

Exit criteria: S1–S5 pass, or findings documented and the plan revised
(fallback: stay on `LegacyRuntime`, revisit).

### Spike findings [2026-06-11] — opencode 1.15.13 (version pin)

- **S1 PASS** — `POST /session?directory=<worktree>` scopes the session;
  file writes landed only in the target directory.
- **S2 BLOCKED (environment, not API)** — only OpenCode Zen/Go credentials
  configured; Zen returned `401 CreditsError` ("Insufficient balance"). No
  direct Anthropic auth available. Needs billing top-up or
  `opencode auth login` (Anthropic) before any real-model validation.
- **S3 ANSWERED (negative)** — `mistral:7b` not installed; tested
  `llama3.2:latest` (3B) via a per-directory `opencode.json` ollama provider
  (`@ai-sdk/openai-compatible`, baseURL `http://127.0.0.1:11434/v1`):
  **0/4 artifact-frontmatter compliance** even with explicit formatting
  instructions, and it emits pretend tool calls as plain text. Local steps
  need a ≥7B tool-capable model; not viable with the currently installed one.
- **S4 PARTIAL** — session create accepts a `permission` ruleset
  (`{permission, pattern, action: allow|deny|ask}`), so
  `GLOBAL_BASH_DENYLIST` maps to `bash`-permission deny rules
  (`Bash(rm *)` → `{"permission": "bash", "pattern": "rm*", "action":
  "deny"}`). End-to-end denial untested (no capable model); the runtime must
  never use `ask` (it parks a pending `/permission` request).
- **S5** — provider failures surface as `info.error` (`APIError`) on the
  assistant message with `statusCode`/`responseBody`, not as a hang:
  credits-exhausted/429 → `RATE_LIMITED`; 401 authentication →
  `AUTH_FAILED`; anything else → `ERROR`. Timeout: client-side timeout +
  `POST /session/{id}/abort` (returns `true`, session goes idle) →
  `TIMEOUT`.
- **S6 DECISION** — synchronous `POST /session/{id}/message` for v1, wrapped
  in Aurora's per-step timeout with abort-on-timeout; `prompt_async`+SSE
  deferred.
- Operational notes: per-directory `opencode.json` is honoured but a cached
  instance must be cleared (`POST /instance/dispose?directory=…`) after
  config changes; the server warns when `OPENCODE_SERVER_PASSWORD` is unset.

**Phase 0 verdict:** exit criteria not fully met (S2/S4 blocked by missing
credits/capable model, not by API gaps). Per the fallback rule: Phases 1–2
proceed (adapter implementation + deployment, fully testable against a mocked
HTTP API), `agent_runtime` **stays `"legacy"` by default**, and Phase 3
(cutover + legacy deletion) is deferred until a funded provider allows a real
end-to-end task run.

## Phase 1 — OpencodeRuntime

- `aurora/service/runtime/opencode.py` — implements `AgentRuntime`:
  - map `AgentDefinition` → session/agent config (model, system prompt from
    role prompt + context files, tool permissions, max turns)
  - create session scoped to `request.workdir`, send artifact-block prompt,
    collect final text, map failures to `Outcome`
  - honour `request.timeout_s`
- `aurora/service/config.py` — `opencode_url: str = "http://127.0.0.1:4096"`;
  `agent_runtime` setting gains `"opencode"`
- `task_runner.py` — runtime factory honours the new setting
- Tests: `aurora/tests/test_opencode_runtime.py` against a mocked HTTP API
  (httpx mock transport), covering outcome mapping and prompt assembly

## Phase 2 — Deployment

- `systemd/opencode.service` — new user unit (pinned version, port, env)
- `scripts/install.sh` — install pinned opencode, enable unit; Ollama remains
  (as an opencode provider backend)
- `README.md` — document the third service and config

## Phase 3 — Cutover and cleanup

- Default `agent_runtime` to `"opencode"`; run a real board task end-to-end
  (create feature/task via Borealis, watch Aurora execute through opencode,
  approve the feature)
- Delete `LegacyRuntime`, `execution/cloud.py`, `execution/local.py`,
  `execution/tools.py`, tool JSON definitions, and their tests; drop
  `claude-agent-sdk` dependency
- `execution/agent_prepare.py` and `execution/artifacts.py` remain (agent
  definitions and artifact contract are Aurora's, not the runtime's)

## Files to Modify

- `aurora/aurora/service/runtime/opencode.py` — new
- `aurora/aurora/service/config.py`, `orchestrator/task_runner.py`
- `systemd/opencode.service` — new; `scripts/install.sh`; `README.md`
- `aurora/tests/test_opencode_runtime.py` — new
- Phase 3 deletions as listed above

## Todo

- [x] 1. Phase 0 spike (S1–S6); record findings + version pin
- [x] 2. Implement `OpencodeRuntime` + tests
- [x] 3. Deployment: systemd unit, install.sh, README
- [ ] 4. End-to-end task run through opencode; flip default — **deferred**:
      blocked on provider credits/capable model (see spike findings)
- [ ] 5. Delete legacy execution path and dependency; full suite + ruff —
      **deferred**: gated on item 4

## Change History

- [2026-06-11] Plan created
- [2026-06-11] Phase 0 spike run against opencode 1.15.13 (findings above).
  Phase 1 implemented: `runtime/opencode.py` (session-per-step, sync message
  API, deny-only permission rules from `GLOBAL_BASH_DENYLIST` +
  `disallowed_tools`, error→Outcome classification, abort-on-timeout);
  `config.py` gains `opencode_url`/`opencode_cloud_provider`/
  `opencode_local_provider`; `build_runtime` gains `"opencode"`. 10 new tests
  in `test_opencode_runtime.py` (httpx MockTransport). Phase 2 implemented:
  `systemd/opencode.service`, install.sh step 4 (pinned via
  `OPENCODE_VERSION`, default 1.15.13) + unit enablement, README services/
  runtime docs. Phases 3–4 deferred pending a funded provider; default
  runtime remains `"legacy"`.

## Dependencies

- Requires 015 (Aurora↔Borealis contract fixes) and 017 (adapter seam).
