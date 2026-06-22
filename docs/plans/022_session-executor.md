# 022 — M1: Session Executor (the spine swap)

Implements milestone M1 of the Build Order in
[021_session-pipeline-architecture.md](021_session-pipeline-architecture.md).
Read that document first — it is the canonical "what and why"; this plan is the
"how" for M1 only.

## Context

Aurora currently drives tasks through a YAML step graph with
confidence-frontmatter routing (`pipeline/executor.py`) over per-step runtime
calls. Per 021, intra-task orchestration moves into opencode sessions; North
keeps the deterministic outer loop. M1 swaps the spine: a **stage runner**
(`run` + `gate` stage types only) executing **session profiles** to completion,
with harness commits at stage boundaries, objective gates from a per-repo
checks manifest, transcript export, and telemetry. The legacy pipeline engine
and cloud/local executors are deleted.

Working agreements (user-confirmed): branch `north-v3`; commit per completed
todo; never push; never touch main. Small design choices 021 doesn't answer:
use judgment, note in Change History, keep moving; stop only for
hard-to-reverse decisions. Live testing: opencode-go subscription models
**sparingly** (smoke only), then local ollama models `llama3.2:3b-65k` and
`mistral:7b-16k` (already configured as an opencode provider in
`~/.config/opencode/opencode.jsonc`; mistral more tool-reliable). Sandbox repo:
`git@github.com:SamP-S/north-test.git` cloned to `~/.north/sandbox/` —
disposable.

## Scope

**Build:**
1. **Session profiles** — `aurora/definitions/profiles/<name>.md`, frontmatter
   (`name`, `description`, `model`, optional `provider`, `permissions`/denied
   tools, optional `agents` roster note) + body = orchestration prompt. Ends
   with the handoff-note request (`## Summary / ## Decisions / ## Concerns /
   ## Suggested status` — requested, never parsed). Replaces
   `agent_prepare.py` agent definitions.
2. **Pipeline v2** — `aurora/definitions/pipelines/<name>.yaml`: `name` +
   ordered `stages:` list; entries `run: {profile}` or
   `gate: {checks: [...], policy: required|if-present}`. Strictly linear, no
   routing keys (anti-DSL rule). Loader + validation.
3. **SessionRunner** (`runtime/session.py`) — `run_session(profile, prompt,
   workdir, timeout) -> SessionResult(outcome, handoff_text, session_id,
   usage, duration)`. Built on the existing opencode HTTP patterns
   (`runtime/opencode.py`): create session with permission rules, post
   message, classify errors (reuse `_classify_api_error` semantics:
   rate-limited / auth / timeout / error), abort on timeout. Plus: **transcript
   export** (GET session messages → `<aurora_home>/transcripts/<project>/
   <task_id>/<session_id>.json`) and token-usage extraction.
4. **Gate executor** (`stages/gates.py`) — load `.north/checks.yaml` from the
   worktree (`check-name: shell command`); run via subprocess in the worktree
   with timeout; **exit code is the only contract**; report = pass/fail per
   check + output tail (~last 80 lines) for the next session. Missing-check
   policy per gate.
5. **Stage runner** (`stages/runner.py`) — walks the stage list; renders each
   `run` stage's opening prompt from a context (task id/title/body, feature,
   accumulated handoff notes, latest gate report); session error → one retry,
   then task `failed`; rate-limited → task `queued` (resume at stage boundary
   later — M1 may restart from stage 0, note as accepted simplification);
   auth → `blocked`; gate fail → `failed`. **Harness commit after each
   successful stage** when the worktree is dirty:
   `[task:<id>][<stage>] <first handoff Summary line or stage name>`.
6. **TaskRunner v2** — same Borealis contract (statuses unchanged); result
   content = handoff notes per stage + gate reports + event log + **session
   manifest** (yaml block: stage, profile, session_id, transcript path,
   tokens, duration).
7. **Default definitions** — profiles `implement.md`, `review.md`; pipeline
   `default.yaml` = `run implement → gate → run review → gate`.

**Retire (delete, with their tests):** `pipeline/executor.py`,
`pipeline/loader.py`, `execution/cloud.py`, `execution/local.py`,
`execution/artifacts.py`, `execution/tools.py`, `execution/agent_prepare.py`,
`runtime/legacy.py`, `runtime/prompts.py`, `runtime/opencode.py` (subsumed by
`session.py`), `models.Artifact`, `TaskState.artifacts` (keep `events`),
old agent/pipeline definition files.

**Untouched:** Borealis entirely; Aurora `review.py`, `git/*`,
`borealis_client.py`, `logctx.py`, supervisor poll loop, API endpoints,
`GLOBAL_BASH_DENYLIST` (maps into session permission rules).

## Files

- New: `aurora/aurora/service/runtime/session.py`,
  `aurora/aurora/service/stages/{__init__,runner,gates,context}.py`,
  `aurora/aurora/service/profiles.py` (loader),
  `aurora/definitions/profiles/*.md`, `aurora/definitions/pipelines/default.yaml`,
  tests mirroring each.
- Modified: `orchestrator/task_runner.py`, `models.py`, `config.py`
  (transcript dir derived from `aurora_home`), `pipeline/` → replaced by v2
  loader (location may move under `stages/`), README.
- Deleted: per Retire list above + their test files.

## Todos

- [x] 1. **Spike (read-only):** probe the live opencode server API — session
      create options (permissions, agent selection), whether message POST
      blocks to completion, response token/usage fields, GET messages shape
      for transcript export, abort semantics. Use only local/ollama or
      no-cost calls. Record findings in Change History; adjust items 3–5 if
      reality disagrees.
- [x] 2. Pipeline v2 loader + validation + tests.
- [x] 3. Profile model + loader + default `implement`/`review` profiles + tests.
- [x] 4. Gate executor: checks manifest, subprocess gates, report rendering,
      missing-check policy + tests (tmp git repos).
- [x] 5. SessionRunner: `run_session` with mocked transport tests (outcomes,
      timeout/abort, usage extraction, transcript export to disk).
- [x] 6. Stage runner: context rendering, run/gate walk, retry-once,
      commit-per-stage (real tmp repos), status mapping + tests.
- [x] 7. TaskRunner v2: wire stage runner, result content with session
      manifest; rewrite `test_full_task_run`/`test_pause_resume`/
      `test_rate_limit` against the new spine.
- [x] 8. Retire legacy modules + dead tests; slim `models.py`; `uv run ruff`
      + full `pytest` green.
- [x] 9. README: profiles/pipelines/gates/checks-manifest sections; document
      the `--first-parent` log idiom (per 021 resolved questions).
- [x] 10. Sandbox: clone `git@github.com:SamP-S/north-test.git` →
      `~/.north/sandbox/north-test`; seed minimal Python project +
      `.north/checks.yaml` (test/lint via uv) + commit & push is NOT allowed —
      leave local (user pushes if wanted).
- [x] 11. **Live smoke #1 (opencode-go, sparingly):** services up (note: after
      plan 020, Borealis expects the board at `~/.north/borealis/board` — set
      `BOARD_PATH` or move the existing clone from `borealis/board`); register
      sandbox project; one trivial feature+task ("add hello.py printing
      hello"); watch full run: session → commit → gate → review → gate → done
      → result with manifest + transcript on disk. Record outcome.
- [x] 12. **Live smoke #2 (local):** same task with `mistral:7b-16k` profile
      variant (fall back to `llama3.2:3b-65k` comparison if cheap); record
      capability notes in Change History.
- [x] 13. Final: plan Change History updated; milestone test verdict recorded;
      commit.

## Verification

- Unit: `uv run pytest` green throughout; `uv run ruff check .` clean.
- Mocked e2e: task dict → TaskRunner v2 → done, with commits in a tmp repo,
  manifest in result, transcript file written.
- Live milestone test = todos 11–12 (the M1 acceptance from 021).

## Loose Ends / Follow-up (post-M1)

Known items deliberately left open at M1 close, for triage into later
milestones or quick fixes.

**Triage update [2026-06-12], post-M2/M3:** §1 **closed** in M2 (plan 023
todo 10 — rate-limit requeues as `ready`, ready re-entry clears `ready_at`
so the cooldown is the backoff). §3 **closed** in M3 (plan 024 todo 4 —
`ensure_managed_clone` + `create_feature_branch` wired into TaskRunner;
proven live: the smoke's `farewell-module` branch was code-created). §6
**half-closed** in M2 (README de-staled; `install.sh` still not
re-validated — it still installs/authenticates Claude Code, which the spine
no longer needs). §4 updated: same services now run M3 code (restarted
2026-06-12); opencode user config gained the borealis MCP server + global
`borealis_*` tool disable. §2 (stage-0 resume), §5 (profile model tuning),
§7 (coarse denylist), §8 (mypy) remain open.

1. **Rate-limited tasks can hot-loop.** Session rate-limit → task `queued` →
   the supervisor re-picks it on the next poll (~5s) and hits the provider
   again. The ready→queued cooldown does not apply to direct `queued` writes.
   Needs a backoff (e.g. requeue as `ready` so the cooldown applies, or a
   `ready_at`-style delay on requeue). Small, worth doing early in M2.
2. **Resume restarts from stage 0** (accepted M1 simplification). Stage-index
   checkpointing is designed (021) but unbuilt — a requeued task repeats
   completed stages, costing tokens. Becomes more important once pipelines
   grow beyond 4 stages or race stages exist (M7).
3. **Feature branch creation is manual.** Nothing in the live path calls
   `create_feature_branch` — the smoke test created `hello-feature` in the
   managed clone by hand, and the managed clone itself was cloned by hand
   (no code clones project repos). Decomposition/feature lifecycle (M3) must
   own branch + clone creation.
4. **Smoke environment left running**: local uvicorn aurora :8000 /
   borealis :8001, opencode serve :4096 (started manually, not via systemd),
   logs in `/tmp/{aurora,borealis,opencode-serve}.log`. Board at
   `~/.north/borealis/board` pushes to a **local bare remote**
   (`board-remote.git`); no real `BOARD_REPO_SSH_URL` configured. Sandbox
   (`~/.north/sandbox/north-test`) and its managed clone hold unpushed
   commits (user pushes if wanted).
5. **Default profile models are a first guess** (`opencode-go/minimax-m3`,
   worked well in smoke #1). Model/provider choice per profile is expected to
   be retuned; telemetry fields for that are in the manifest already.
6. **README Requirements section is stale** (mentions Claude Code CLI and
   `codellama:7b` pull — neither is part of the new spine). `install.sh` not
   re-validated against the slimmed config surface.
7. **`GLOBAL_BASH_DENYLIST` is coarse** (`rm`, `curl`, `wget` denied outright)
   — fine for smoke, will annoy real implement sessions that legitimately
   need them. Revisit alongside per-profile permission design in M2/M3.
8. **mypy debt** (plan 020 item 15): strict-mode errors remain; CI job stays
   informational. The new `stages/`/`runtime/session.py` code is typed and
   should be kept clean for when it flips to required.

## Change History

- [2026-06-12] Plan created from 021 M1 scope + discussion detail (handoff
  sections, exit-code-only gates, commit-per-stage format, retirement list,
  token-frugality and sandbox agreements).
- [2026-06-12] Todo 1 spike done (opencode 1.15.13, live server, ollama-only
  calls). Findings:
  - Endpoints used by the old runtime still exist: `POST /session`,
    `POST /session/{id}/message`, `POST /session/{id}/abort`,
    `GET /session/{id}/message`. New since then: `prompt_async`, session
    `permissions/{permissionID}` reply, `/session/{id}/todo`, `/agent` list.
  - `POST /session` body accepts `title`, `agent`, `model{id, providerID}`,
    `permission: [{permission, pattern, action}]` — the existing
    `_permission_rules` shape is still valid. `directory` is a query param.
  - **Message POST blocks to completion** and returns
    `{info: AssistantMessage, parts: [...]}`. Verified live with
    `ollama/llama3.2:3b-65k` (41s round trip).
  - Usage fields on `info`: `tokens{total, input, output, reasoning,
    cache{read, write}}`, `cost`, `time{created, completed}`, `finish`.
  - Errors: `info.error` is `{name, data:{message,...}}` with names
    `ProviderAuthError | UnknownError | MessageAbortedError | APIError |
    ContextOverflowError | MessageOutputLengthError | StructuredOutputError`.
    `_classify_api_error` semantics carry over; classify on `name` first
    (ProviderAuthError → auth), then statusCode/message heuristics.
  - `GET /session/{id}/message` → `[{info, parts}]` — exactly the transcript
    export shape needed; parts include `step-start/step-finish/text/tool`.
  - Abort: `POST .../abort` → `true`; the in-flight assistant message is
    finalized with `error.name=MessageAbortedError`.
  - Live providers: `opencode-go`, `opencode`, `ollama`. The configured
    default `opencode_cloud_provider="anthropic"` **does not exist** on this
    server — profiles must carry explicit provider ids; settings default to
    be revisited when wiring smoke tests (noted, not blocking).
  - No plan changes needed for items 3–5; reality matches the assumed shape.
- [2026-06-12] Todo 2 done: pipeline v2 in `service/stages/pipeline.py`
  (per-plan option to move under `stages/`). Judgment call: no per-stage
  failure-policy key in M1 YAML — the runner hardcodes the retry-once /
  fail-task semantics, keeping routing out of config (anti-DSL); revisit
  when escalate-vs-retry needs to differ per stage. Strict validation:
  single-key stage mappings, unknown keys rejected, gate policy
  `required|if-present`. 12 tests.
- [2026-06-12] Todo 3 done: `service/profiles.py` (strict loader — unlike
  agent_prepare's warn-and-skip, a bad profile referenced by a pipeline is an
  error). Defaults `implement.md`/`review.md` target `opencode-go/minimax-m3`
  (judgment: arbitrary capable go-plan pick; confirm/adjust at smoke #1).
  Both deny `git commit`/`git push` (harness owns commits); review also
  denies write/edit. `default.yaml` gates use `policy: if-present` so the
  default pipeline works on repos without a checks manifest. 9 tests.
- [2026-06-12] Todo 4 done: `stages/gates.py`. Judgment calls: an unreadable/
  malformed manifest fails the gate deterministically (failed CheckResult
  carrying the parse error — self-explanatory to the next session) rather
  than raising; per-check timeout (default 600s) is a failed check, not an
  exception; output tail = last 80 lines of stdout+stderr. 12 tests.
- [2026-06-12] Todo 5 done: `runtime/session.py` (SessionRunner,
  SessionResult, SessionUsage, `permission_rules`). Judgment calls: caller
  passes `transcript_dir` (stage runner owns the `<project>/<task>` layout;
  session runner only writes `<session_id>.json`); transcript exported
  best-effort on every outcome incl. timeout (forensics); profile without
  `provider` falls back to `settings.opencode_local_provider` (local-first);
  error classification keys on error `name` first (ProviderAuthError), then
  the legacy statusCode/message heuristics. 10 tests.
- [2026-06-12] Todo 6 done: `stages/context.py` (StageContext + prompt
  rendering; prompt explicitly tells sessions git is the source of truth) and
  `stages/runner.py` (StageRunner + StageRecord audit trail). Judgment calls:
  stage label = `<index>-<profile>` / `<index>-gate` (disambiguates repeated
  profiles in commit messages); rate-limited gets **no** retry (retry would
  burn the same limit — straight to `queued`); auth → `blocked`,
  error/timeout → retry once → `failed`; gates also commit-if-dirty (e.g.
  formatters run by checks); missing/invalid profile → `blocked` (config
  problem, not task failure); summary-line use in commit subjects is cosmetic
  only (no routing). M1 simplification confirmed: resume restarts from stage
  0. 11 tests.
- [2026-06-12] Todo 7 done: TaskRunner v2 rewritten on the stage spine;
  result = handoff notes + gate reports + session-manifest yaml block
  (stage, profile, session_id, transcript, tokens, cost, duration_s,
  attempts, commit) + event log. `Settings.transcripts_dir` property added
  (derived from `aurora_home`). Feature description (when fetched for the
  branch) feeds StageContext. Integration tests rewritten:
  `test_full_task_run` is now a true mocked e2e (real tmp-repo worktree,
  real Stage/SessionRunner over httpx MockTransport whose first message
  call writes hello.py → verifies harness commit, 2 transcripts on disk,
  manifest in result); `test_pause_resume` covers block-before-stages paths
  + queued propagation; `test_rate_limit` drives a 429 through the real
  spine. 215 tests green.
- [2026-06-12] Todo 8 done: deleted `pipeline/` (whole package),
  `execution/{cloud,local,artifacts,tools,agent_prepare}.py` (kept
  `policy.py`), `runtime/{legacy,prompts,opencode}.py`,
  `definitions/agents/`, `definitions/tools/`, `map-code-review.yaml`, and 8
  legacy test files (54 tests). `runtime/__init__` slimmed to `Outcome`;
  `models.py` slimmed (Artifact, Provider, `TaskState.artifacts`,
  `current_step`, `step_attempts` removed — events kept); config drops
  `agent_runtime`, `opencode_cloud_provider`, `ollama_base_url`,
  `ollama_default_num_ctx` (verified unreferenced). 161 tests green; service
  app imports clean.
- [2026-06-12] Todo 9 done: README "Agent runtime" section replaced with
  "Session execution" (profiles, pipelines, gates/checks manifest, result
  contents/transcript paths, `--first-parent` idiom); intro + repo-layout
  lines updated. Verified no `AGENT_RUNTIME` references remain anywhere
  (scripts/, systemd/, README).
- [2026-06-12] Todo 10 done: north-test was an empty repo; seeded
  `north_test/calc.py` + passing test + `.north/checks.yaml`
  (`test: uv run pytest -q`, `lint: uv run ruff check .`), hatchling build
  backend so `uv run pytest` installs the package. Both checks verified
  green locally. Two local commits, not pushed.
- [2026-06-12] Todo 11 in progress — live-wiring fix: `main.py` passed
  `<repo>/aurora/aurora` as `aurora_path`, but `definitions/` lives at
  `<repo>/aurora/` (pre-existing latent bug, exposed by the first live
  pipeline load; the legacy loader used the same convention and would have
  hit it too). Fixed to `parent.parent.parent`. Smoke env: no board existed
  anywhere (neither `~/.north/borealis/board` nor `<repo>/borealis/board`) —
  created a local bare `~/.north/borealis/board-remote.git` + clone as the
  board (push target stays local); managed clone
  `~/.north/aurora/repos/north-test` cloned from the sandbox; feature branch
  `hello-feature` created manually in the managed clone (branch creation is
  outside M1 scope). Bypassed the 300s ready-cooldown by patching the task
  straight to `queued`.
- [2026-06-12] Todo 11 done — **live smoke #1 PASSED** (task 001, pipeline
  `default`, `opencode-go/minimax-m3`): implement session (30.6s, 10,550
  tokens, $0.0011, 10 tool calls) wrote hello.py + test, harness committed
  `[task:001][0-implement] Added \`hello.py\` ...`; gate PASS (build skipped
  if-present, lint+test exit 0); review session verified the diff (read
  files itself, ran checks, flagged the packaging nuance correctly); gate
  PASS; task → done. Result file carries both handoff notes, both gate
  reports, and the session manifest with working transcript paths; both
  transcripts on disk (5/7 messages, 10/11 tool calls). Total go-plan spend:
  2 sessions, ~21k tokens, ~$0.002. Handoff-note sections followed exactly.
- [2026-06-12] Todo 12 done — **live smoke #2 (local) exposed and closed a
  real gap.** New definitions: `implement-local`/`review-local` profiles
  (`ollama/mistral:7b-16k`) + `local.yaml` pipeline (committed as the local
  variants). First run: mistral produced **zero work** — both sessions
  echoed tool documentation instead of using tools (no file writes, no
  commits) — yet the task went `done` because an unchanged repo trivially
  passes its own gates. That is exactly 021's "diff non-empty" done-gate,
  which M1 had not implemented. **Fix:** stage runner now fails a walk that
  completes with zero harness commits (`test_no_commits_fails_done_gate`).
  Re-run live: same mistral behavior, task now honestly lands `failed`
  (94s/115s per session, ~8.7k tokens each, attempts 1, commits null —
  manifest tells the whole story). Capability note: `mistral:7b-16k` is not
  viable as a session orchestrator seat (fails the tool loop entirely);
  `llama3.2:3b-65k` comparison skipped (strictly weaker model, no new
  information for the time cost). Local models remain candidates for
  subagent seats under a capable orchestrator, per 021's core trade.
- [2026-06-12] Todo 13 — **M1 MILESTONE VERDICT: PASSED.** The acceptance
  test from 021 ("real task runs implement → gate → review → gate end to end
  with commits and linked transcript") was demonstrated live (task 001,
  smoke #1). The spine swap is complete: legacy pipeline engine, cloud/local
  executors, artifact/confidence parsing, and legacy runtime are deleted
  (−2,080 lines); the new spine is profiles + pipeline v2 + SessionRunner +
  gates + stage runner + TaskRunner v2 (162 tests, ruff clean). Accepted M1
  simplifications carried forward: resume restarts from stage 0; no
  per-stage failure-policy config; feature-branch creation still manual
  (pre-existing gap, not M1 scope). Loose ends for the user: smoke services
  left running locally (uvicorn :8000/:8001, opencode :4096, logs in
  /tmp/{aurora,borealis}.log); board lives at ~/.north/borealis/board with a
  local bare remote; sandbox + managed clone have unpushed local commits;
  default profiles point at opencode-go/minimax-m3 (worked well; revisit
  model choice freely).
- [2026-06-12] Post-milestone closeout: "Loose Ends / Follow-up" section
  added (8 items, triaged); M1 learnings written back into 021's Change
  History; plan 020 annotated with the review items M1 retired (10, 11, 13,
  16) vs still open (12, 14, mypy). Plan 023 (M2: board extensions + MCP)
  written, with the rate-limit-requeue fix (loose end §1) and README
  staleness (§6) pulled into its todos, and a kickoff prompt appended for a
  fresh context.
