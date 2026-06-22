# 024 — M3: Decomposition Loop

Implements milestone M3 of the Build Order in
[021_session-pipeline-architecture.md](021_session-pipeline-architecture.md)
("Conversation Intake & Decomposition" is the governing section). M1 (022)
and M2 (023) are complete: the session spine works end to end and the board
has conversations, drafts, promote verbs, and the MCP surface. M3 closes the
loop: pending conversations are decomposed by an Aurora session into draft
features/tasks, grounded in the project repo, with durable decisions
distilled into repo docs under a deterministic docs-only guard.

## Context

Working agreements (user-confirmed, carried from M1/M2): branch `north-v3`;
commit per completed todo; never push; never touch main; update this plan's
checkboxes + Change History as you go; `uv run ruff check .` +
`uv run pytest` after each todo; small unanswered design choices → judgment
+ note here; stop only for hard-to-reverse decisions. Tokens: decompose
prompt iteration uses opencode-go sparingly (smoke only); local models are
not orchestrator seats (M1 finding). Live env: manual uvicorn aurora :8000 /
borealis :8001, opencode :4096; board `~/.north/borealis/board` (local bare
remote); sandbox `~/.north/sandbox/north-test`, managed clone
`~/.north/aurora/repos/north-test`. MCP endpoints live at
`http://127.0.0.1:8001/mcp/{grant}/` (trailing slash).

021's named behavior-risk hotspot for M3: **decompose prompt quality** —
budget iteration time on the profile, not just code.

## Scope

**Build:**

1. **Borealis enablers (small):**
   - Optional `decomposed_from` on task/feature create (REST body + MCP
     create tools + frontmatter + GET exposure).
   - Allow `decomposing → pending` transition (failed decompose returns the
     conversation to the queue; forward-only otherwise).
2. **Aurora BorealisClient additions:** `get_pending_conversations()`,
   `get_conversation(project, id)`, `update_conversation_status(project,
   id, status, decomposed_into=None, result_content=None)`,
   `list_features(project)`, `list_feature_tasks(project, feature)` (for
   the before/after board diff).
3. **Repo lifecycle (closes 022 loose end §3):**
   - `ensure_managed_clone(project, ssh_url)` — clone if missing.
   - TaskRunner: create the feature branch from `base_branch` if it does
     not exist (instead of blocking) — branch creation owned by code, not
     by hand.
4. **ConversationRunner** (`orchestrator/conversation_runner.py`):
   pick → `decomposing` → ensure clone → ephemeral worktree on
   `base_branch` → render decompose prompt (conversation body + project
   context + MCP usage instructions) → `run_session(decompose profile)` →
   **docs-only diff guard** (allowlist `docs/`, `AGENTS.md`): commit to
   `base_branch` if clean, discard + note if violated (deterministic — the
   session cannot land code on main) → board diff → conversation PATCH
   `decomposed` + `decomposed_into` + result (handoff note + session
   manifest, same shape as task results) → remove worktree.
   Failure mapping: session error/timeout/rate-limit → conversation back
   to `pending` + result note; supervisor keeps an in-memory failure
   backoff (skip recently-failed conversations for a few polls — pending
   has no cooldown machinery, avoid a hot loop).
5. **Two-queue supervisor:** poll pending conversations first; only when
   empty take project tasks (then auto-approve reviews). Decompose runs
   surface via the same active-events API.
6. **Decompose profile + MCP wiring:** `definitions/profiles/decompose.md`
   (opencode-go/minimax-m3 first guess; iterate at smoke). opencode user
   config gains a `borealis` remote MCP server pointed at
   `/mcp/decomposer/`; implement/review profiles deny board-MCP tools by
   pattern (per-profile grants, 021). Verify live how opencode namespaces
   MCP tool names before hardcoding deny patterns.
7. **Docs/README:** decomposition flow, decompose profile, conversation
   lifecycle incl. failure path, repo-lifecycle notes.

**Out of scope (resist):** cockpit/condensing UX and notifications (M4),
racing decomposition + apply sessions (M7), thread-content injection into
implement sessions (defer unless free), `auto_ready` (021 names it a later
extension), execution environments.

## Files

- New: `aurora/aurora/service/orchestrator/conversation_runner.py`,
  `aurora/definitions/profiles/decompose.md`,
  `aurora/tests/test_conversation_runner.py` (+ docs-only-guard tests with
  real tmp repos), `aurora/tests/test_two_queue.py`.
- Modified: Borealis `api/{tasks,features,conversations}.py` +
  `service/mcp.py` + `models.py`/`parser.py` (decomposed_from,
  decomposing→pending) + tests; Aurora `borealis_client.py`,
  `git/features.py` (ensure clone, ensure branch), `orchestrator/
  {supervisor,task_runner}.py` + tests; `~/.config/opencode/opencode.jsonc`
  (live env, not in repo); README.

## Todos

- [x] 1. **Recon (read-only, live):** opencode 1.15.13 MCP support — config
      shape for remote MCP servers, how MCP tools are named in sessions,
      whether permission rules can deny them by pattern; confirm with a
      no-cost local probe. Record findings; adjust todo 6 if reality
      disagrees.
- [x] 2. Borealis enablers: `decomposed_from` + `decomposing→pending` +
      MCP create-tool passthrough + tests.
- [x] 3. Aurora client additions + tests.
- [x] 4. Repo lifecycle: ensure-clone + ensure-feature-branch in TaskRunner
      + tests (tmp bare remotes).
- [x] 5. ConversationRunner: prompt rendering, session run, docs-only guard
      (real tmp repos: clean commit, violation discard), board-diff
      bookkeeping, status/result reporting, failure-to-pending + tests.
- [x] 6. Two-queue supervisor + failure backoff + tests; decompose profile
      first draft; opencode MCP config on the live box; deny patterns on
      implement/review profiles.
- [x] 7. README/docs update.
- [x] 8. **Live smoke (the M3 milestone test, opencode-go sparingly):**
      ship a real conversation ("add a farewell module with tests" style)
      → decomposer creates draft feature+tasks grounded in the sandbox
      repo → human-promote via CLI → tasks execute through the M1 spine →
      feature reaches review. Iterate the decompose prompt if the first
      breakdown is poor (budgeted). Record outcome + costs.
- [x] 9. Final: change history complete; milestone verdict recorded; commit.

## Verification

- `uv run pytest` green throughout; `uv run ruff check .` clean.
- Milestone test (021 M3): conversation → drafts → promote → execute →
  review, demonstrated live on the sandbox.
- Docs-only guard proven by test to reject a code-touching decompose diff.

## Loose Ends / Follow-up (post-M3)

Known items deliberately left open at M3 close:

1. **Decomposer may task-out docs distillation** instead of writing `docs/`
   directly (observed in the live smoke: handoff claimed a docs write, the
   guard truthfully logged "no repo changes", and the convention became
   task 002). Defensible — but if direct distillation matters, iterate the
   decompose prompt; the guard + result file make the choice auditable.
2. **Failed-attempt forensics are overwrite-prone** — each conversation
   status PATCH replaces the companion result file, so a success erases the
   previous failure note. Transcripts on disk remain the durable record
   (`transcripts/<project>/conv-<id>/`); consider append-or-archive
   semantics if failure history starts mattering.
3. **Decompose backoff is in-memory only** — an Aurora restart clears it
   (mitigated by startup recovery for stranded `decomposing` conversations,
   and the failure note in the result). Fine until conversations fail
   persistently.
4. **No per-conversation budget** — a pathological decompose could burn its
   full 1800s repeatedly (300s backoff between attempts, forever). Budget
   caps are a 021 known gap (needs telemetry); decompose should be included
   when they land.
5. **Orphaned opencode sessions are not aborted** — if Aurora dies
   mid-decompose, the session keeps running server-side until completion or
   idleness (observed: it stalled quickly once the HTTP caller vanished).
   Startup recovery fixes the board state but doesn't abort the session.
6. **Smoke environment** unchanged from 022 §4 (manual uvicorn/nohup), now
   on M3 code; opencode config carries the borealis MCP + global tool
   disable (backup at `~/.config/opencode/opencode.jsonc.bak-m3`).

## Change History

- [2026-06-12] Plan created from 021 M3 scope + M2 learnings (MCP endpoint
  shape, trailing slash, grant sets) + 022 loose end §3 (clone/branch
  creation) pulled in as todo 4.
- [2026-06-12] Todo 1 recon done (live opencode 1.15.13, zero cost):
  - Remote MCP config verified live: `"mcp": {"borealis": {"type":
    "remote", "url": "http://127.0.0.1:8001/mcp/decomposer/"}}` added to
    `~/.config/opencode/opencode.jsonc` (backup `.bak-m3`); after restart
    `GET /mcp` reports `borealis: connected` — the M2 surface speaks
    opencode's client dialect out of the box.
  - MCP tools are named `<server>_<tool>` (e.g. `borealis_create_task`);
    they do not appear in `/experimental/tool/ids` (built-ins only) —
    injected at message time.
  - **Better hook than expected:** the message POST body accepts
    `tools: {name: bool}` (additionalProperties boolean), alongside
    `system`/`model`/`parts` already used by SessionRunner. Plan
    adjustment for todo 6: per-profile MCP grants = profile frontmatter
    gains a `tools` map passed through to the message POST
    (decompose: `{"borealis_*": true}`), with `"borealis_*": false` set
    globally in opencode.jsonc so non-decompose seats never see board
    tools — instead of fragile deny-pattern permission rules. Wildcard
    support in the per-message map to be confirmed with a cheap local
    probe in todo 6.
- [2026-06-12] Todo 2 done: `decomposed_from` optional on task/feature
  create (REST + MCP tools + frontmatter round-trip + GET exposure, ids
  zero-padded on parse); `decomposing → pending` transition allowed
  (failed-decompose retry; otherwise forward-only). 2 new tests
  (216 total green).
- [2026-06-12] Todo 3 done: BorealisClient gains
  `get_pending_conversations`, `get_conversation`,
  `update_conversation_status` (retried like task status — a lost report
  strands a conversation in `decomposing`), `list_features`,
  `list_feature_tasks`. 2 tests via MockTransport (218 total green).
- [2026-06-12] Todo 4 done (closes 022 loose end §3): `ensure_managed_clone`
  (idempotent clone from project ssh_url) and TaskRunner now fetches the
  project (ssh_url/base_branch), ensures the clone, and calls the
  previously-orphaned `create_feature_branch` (create-or-adopt) before the
  worktree — clone and branch creation are code-owned. Unpreparable
  branch/clone → blocked(`infra`). 2 new tests; integration fixtures gained
  get_project + lifecycle monkeypatches (220 total green).
- [2026-06-12] Todo 5 done: `orchestrator/conversation_runner.py`. Judgment
  calls: decompose worktree is **detached** at base_branch
  (`create_detached_worktree` added — the clone holds base_branch checked
  out, a branch worktree would collide); docs commit lands as a detached
  commit then `merge --ff-only` into base_branch in the clone; guard
  allowlist from `Settings.docs_allowlist` (`docs/`, `AGENTS.md`); on
  violation the **whole** repo diff is discarded (not just the offending
  files — a half-applied distillation is worse than none) while board
  writes stand; `git status --porcelain -uall` so untracked files report
  individually; `decomposed_into` computed by harness-side board diff
  (before/after feature+task listing — deterministic, never trusts the
  session's self-report; `decomposed_from` stamps are advisory). Failure
  mapping: every non-OK outcome (incl. auth — conversations have no
  blocked state) → back to `pending` with the failure + manifest in the
  result file. 5 tests incl. real-repo guard cases (225 total green).
- [2026-06-12] Todo 6 done. Per-profile tool grants: profile frontmatter
  gains a `tools:` map (validated str→bool), SessionRunner passes it as
  the message-POST `tools` field; `decompose.md` profile carries
  `"borealis_*": true` and the live opencode.jsonc now sets a global
  `"tools": {"borealis_*": false}` — non-decompose seats never see board
  tools, no per-profile deny patterns needed (cleaner than the planned
  deny approach). Two-queue supervisor: `decompose_next_conversation`
  runs before the task poll (auto-approve only when both queues idle);
  in-memory 300s backoff per failed conversation (pending has no cooldown
  machinery), crash-contained. Wildcard behavior of the per-message tools
  map deliberately left to the live smoke (a local-model probe can't
  observe the tool list reliably; minimax-m3 must call borealis tools for
  the smoke to pass, which is the real test). 8 new tests
  (234 total green).
- [2026-06-12] Todo 7 done: README "Decomposition" section (two queues,
  docs-only guard, board-diff bookkeeping, failure backoff, on-demand
  clones/branches) + session-profile `tools:` map documented.
- [2026-06-12] Todo 8 — **live smoke PASSED on attempt 3**; two real bugs
  found and fixed by attempts 1–2 (this is what live smokes are for):
  - Attempt 1: decompose ran beautifully (MCP wildcard grant **confirmed
    live** — `borealis_list_features/list_tasks/get_task` all called) but
    timed out at 900s mid-exploration: minimax-m3 is slow per step and the
    prompt invited wandering (it read other worktrees by absolute path).
    Fixes: `Settings.decompose_timeout_s=1800`; prompt tightened ("work
    briskly", "stay inside this worktree"). Failure path worked: back to
    `pending` with manifest in the result.
  - Attempt 2 exposed a **stale-`now` backoff bug**: backoff was computed
    from the loop-entry monotonic time, which a 900s session made 900s
    stale — the failed conversation was re-picked 5s later; my service
    restart then orphaned that session, stranding the conversation in
    `decomposing` (nothing recovers it). Fixes: backoff recomputes
    `time.monotonic()` after the run; new startup
    `recover_stale_conversations` (decomposing→pending, mirror of stale
    task recovery, verified live in attempt 3's log); decompose worktree
    force-recreated per attempt (a dead attempt's worktree carried stale
    state). +1 test (235 green). Junk conversation 002 retired via the
    API's decomposing→decomposed path.
  - Attempt 3, end to end in ~9 min: decompose 66.5s/15.0k tok/$0.0016 —
    created `farewell-module` + 2 draft tasks (`decomposed_from: "003"`,
    sensible dep 001→002, self-sufficient bodies; handoff Concerns
    correctly flagged the conversation's reference to a nonexistent hello
    test). Human gate: CLI `feature promote` + `task promote` ×2; cooldown
    bypassed via ready→queued PATCH. Both tasks executed through the M1
    spine in a **code-created** branch/worktree (022 §3 closed live),
    commits landed, gates green, feature → review. Total spend ~$0.006 /
    ~62k tokens across 5 sessions. Quirk noted: the decomposer put the
    docs distillation into task 002 instead of writing docs/ directly
    (handoff text slightly overclaims; harness log `docs guard: no repo
    changes` tells the truth) — defensible, the guard + result file made
    it visible; prompt iteration can push harder later if direct
    distillation matters.
- [2026-06-12] Todo 9 — **M3 MILESTONE VERDICT: PASSED.** The acceptance
  test from 021 ("conversation → drafts → promote → execute → review") was
  demonstrated live end to end. Net new: two-queue supervisor
  (conversations first) with failure backoff and stale-conversation
  recovery; ConversationRunner with docs-only guard and harness-side
  `decomposed_into` bookkeeping; per-profile tool grants (message-POST
  `tools` map) carrying the board-MCP grant to exactly one seat;
  code-owned clone/branch lifecycle; `decomposed_from` provenance;
  `decomposing→pending` retry transition. 235 tests, ruff clean.
  Loose ends for later: decomposer prefers tasking-out docs distillation
  over writing docs/ directly (prompt iteration, M4+ when it matters);
  thread-content injection into session context still open (was already
  M3-deferred-to-M3 in 023 — now explicitly M4 cockpit-adjacent or
  whenever blocked-question flow gets exercised for real); board junk on
  the live sandbox (m2-smoke blocked tasks, hello-feature in review)
  left as-is; smoke services still manual uvicorn/nohup.

## Appendix — Kickoff prompt for a fresh context

```
We're continuing North's v3 architecture. Setup context:

1. Read docs/plans/021_session-pipeline-architecture.md — the canonical
   direction doc. Trust it over anything else.
2. Skim docs/plans/022_session-executor.md and 023_board-extensions-mcp.md
   Change Histories (M1+M2 complete) for live-env state and agreed
   simplifications.
3. Read docs/plans/024_decomposition-loop.md — the M3 implementation plan.
   Execute it now, working through the todos in order.

Working rules (same as M1/M2):
- Branch north-v3 (already checked out). Commit per completed todo. Never
  push. Never touch main.
- Update plan 024's todo checkboxes and Change History as you go; run
  "uv run ruff check ." and "uv run pytest" after each todo.
- Small design choices 021 doesn't answer: use your judgment, note it in
  the plan's Change History, keep moving — stop only for hard-to-reverse
  decisions.
- Tokens: opencode-go subscription models sparingly (smoke + decompose
  prompt iteration only). Local models are not orchestrator seats.
- Live env: aurora :8000, borealis :8001, opencode :4096 (manual
  uvicorn/nohup, logs /tmp/). Board ~/.north/borealis/board (local bare
  remote). Sandbox ~/.north/sandbox/north-test; managed clone
  ~/.north/aurora/repos/north-test. MCP: http://127.0.0.1:8001/mcp/{grant}/
  (trailing slash). Restart services freely after code changes.

Cook through as far as you can get.
```
