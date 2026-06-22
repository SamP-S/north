# 023 — M2: Board Extensions + MCP Surface

Implements milestone M2 of the Build Order in
[021_session-pipeline-architecture.md](021_session-pipeline-architecture.md).
Read 021 first (especially "Board Extensions (Borealis)", "MCP Strategy",
"Conversation Intake & Decomposition", "Named Principles") — it is canonical.
M1 (plan 022) is complete: the session spine works end to end. M2 is almost
entirely Borealis-side; Aurora's loop is untouched (the two-queue supervisor
is M3).

## Context

The board grows the objects and verbs the decomposition loop (M3) and the
cockpit (M4) will need: conversations as first-class board objects, comment
threads, `blocked_reason`, server-enforced draft + promotion, the `split`
verb, and a Borealis MCP surface beside (never instead of) REST. Two named
principles govern every design choice here: **single-writer board mutation**
and **REST stays canonical; MCP is a surface, not the spine**.

Working agreements (user-confirmed, carried from M1): branch `north-v3`;
commit per completed todo; never push; never touch main; update this plan's
checkboxes + Change History as you go; `uv run ruff check .` + `uv run pytest`
after each todo; small unanswered design choices → judgment + note here;
stop only for hard-to-reverse decisions. Live testing uses the local smoke
environment from M1 (see plan 022 "Loose Ends" §4 for its state) — board API
testing needs no model tokens at all; MCP smoke can use a Claude Code or
opencode session pointed at the MCP endpoint.

## Scope

**Build (all Borealis unless noted):**

1. **Conversations** — first-class board objects at
   `projects/<name>/conversations/<id>.md` (frontmatter + body, same
   parser/writer machinery as tasks). Frontmatter: `status: pending |
   decomposing | decomposed`, `created_at`, `source: voice|text`, and on
   completion `decomposed_into:` (list of created feature/task refs).
   Companion `<id>.result.md` for the decompose session's handoff note.
   REST: `POST /api/projects/{p}/conversations` (create = ship),
   `GET` list/detail, `PATCH` status with transition validation
   (pending→decomposing→decomposed only; no deletes via API — drafts are
   deletable, conversations are audit).
2. **Pending-conversation queue** — `GET /api/conversations/pending` served
   alongside the task queue (global, like `/api/queue`), ordered by
   `created_at`. M3's supervisor will poll it first; M2 only serves it.
3. **Comment threads** — append-only companion files: `<task>.thread.md`
   (sibling to `<task>.result.md`) and `_feature.thread.md`. Typed entries
   `[question]/[answer]/[note]` with author + timestamp. REST:
   `GET/POST .../tasks/{id}/comments` and `.../features/{f}/comments`.
   Append-only: no edit/delete endpoints, parser tolerates hand-edited files.
4. **`blocked_reason`** — task frontmatter distinguishing
   `question` (blocked-on-human) from `dependency`. Posting an `[answer]`
   comment to a question-blocked task flips it back to `ready` (one board
   commit). Aurora touch (small): TaskRunner sets `blocked_reason` when it
   blocks a task, clears on requeue; thread content injection into session
   context is M3 — M2 only stores and serves it.
5. **Server-enforced draft + promotion** — creation endpoints already land
   tasks as `draft`; extend to features; add explicit promote verb
   (`POST .../promote`, draft→ready for tasks, draft→open for features) and
   **server-side transition gating** on the status PATCH (reject illegal
   jumps like draft→done; sysadmin escape via existing CLI direct writes is
   acceptable). Statuses: add `superseded` to `TaskStatus` (for split),
   `draft` to `FeatureStatus`.
6. **`split` verb** — `POST .../tasks/{id}/split` with replacement task
   bodies; atomically (one board commit): create children (inherit parent's
   `depends_on`, carry `split_from: <parent>`), re-point dependents of the
   parent to all children, set parent `superseded` (kept for audit). Reject
   splitting tasks in terminal states or with in-progress work.
7. **Borealis MCP server** — FastMCP (or equivalent) mounted beside FastAPI
   on the same process, loopback-only. Curated tool subset calling the same
   service layer as REST (never bypassing it): read tools (queue, features,
   tasks, review list, conversations) + write verbs (`create_conversation`,
   `add_comment`, `promote_draft`, `create_feature`, `create_task`,
   `split_task`). **Per-profile tool filtering**: named grant sets
   (`decomposer`: read + create verbs; `implementer`: comment + split;
   `reviewer`/`cockpit`: read + comment + promote + create_conversation),
   selected by a token/header the MCP client presents. Token auth =
   defense-in-depth, not exposure enabler (021).
8. **Notifier groundwork only if free** — M2 does NOT build notifications
   (M4); but new state changes (conversation shipped, task blocked on
   question) should flow through one internal event call-site so M4 has a
   seam. Judgment call on shape; keep it one function, not a framework.

**Out of scope (resist):** decomposition sessions and the two-queue
supervisor (M3), notification transports (M4), voice (M5), memory (M6),
race/vote (M7). No Aurora changes beyond `blocked_reason` stamping.

## Files

- New: `borealis/borealis/service/board/conversations.py` (or folded into
  parser/writer — judgment), `borealis/borealis/service/api/conversations.py`,
  `borealis/borealis/service/api/comments.py`,
  `borealis/borealis/service/mcp.py`, tests mirroring each
  (`borealis/tests/test_conversations.py`, `test_comments.py`,
  `test_split.py`, `test_draft_gating.py`, `test_mcp.py`).
- Modified: `borealis/borealis/service/models.py` (ConversationModel,
  ThreadEntry, `superseded`, feature `draft`, `blocked_reason`),
  `board/parser.py`, `board/writer.py`, `board/loader.py`,
  `api/tasks.py` (split, promote, transition gating, comments wiring),
  `api/features.py` (draft, promote, comments), `main.py` (routers + MCP
  mount + pending-conversations endpoint), `cli/` (conversation/comment/
  promote/split commands — keep thin), root `pyproject.toml` (fastmcp dep),
  Aurora `orchestrator/task_runner.py` (+ its tests) for `blocked_reason`.
- README: conversations, threads, draft lifecycle, split, MCP surface +
  grant sets.

## Todos

- [x] 1. **Recon (read-only):** map `board/{parser,writer,loader}.py` and the
      existing file conventions (frontmatter fields, commit message format,
      atomicity helpers); confirm how `api/tasks.py` builds `BoardCtx`;
      pick the MCP library (FastMCP vs `mcp` SDK — must mount inside the
      existing uvicorn/FastAPI process) and verify it imports under the
      workspace Python. Record findings + any scope adjustments here.
- [x] 2. Models + parser/writer: `superseded`, feature `draft`,
      `blocked_reason`, `split_from`, ConversationModel + conversation
      file round-trip, ThreadEntry + thread-file append/parse. Unit tests
      (tmp board repos, same pattern as existing parser tests).
- [x] 3. Conversations REST: create/list/detail/status-transition endpoints +
      pending queue endpoint + board commits + tests.
- [x] 4. Comment threads REST: task + feature `GET/POST comments`, typed
      entries, append-only writer + tests.
- [x] 5. `blocked_reason` + answer-flips-ready: status PATCH stamps/clears
      reason; `[answer]` on question-blocked task → `ready` in one commit;
      Aurora TaskRunner stamps `blocked_reason` (auth → `question`? No:
      auth/infra → `infra`, judgment) + tests both sides.
- [x] 6. Draft enforcement: feature draft status, promote verbs, server-side
      transition table for task + feature status PATCH + tests (including
      "decomposer-created items always land draft" contract).
- [x] 7. Split verb: atomic relink + `superseded` + guards + tests
      (dependents re-pointed, deps inherited, one commit, idempotency-safe
      failure).
- [x] 8. MCP server: mount, tool definitions calling the service layer,
      grant-set filtering by token, loopback binding + tests (tool list per
      grant set; a write through MCP produces the same board commit as REST).
- [x] 9. CLI: `borealis conversation create/list/show`, `comment add/list`,
      `task promote`, `feature promote`, `task split` (thin wrappers) + tests.
- [x] 10. Quick fix from 022 loose ends §1: rate-limited tasks requeue as
      `ready` (cooldown applies) instead of `queued` — Aurora-side one-liner
      + test. (Small, high value, fits M2's board-discipline theme.)
- [x] 11. README + docs: new objects/verbs/MCP grants documented; update
      stale Requirements lines (022 loose ends §6).
- [x] 12. **Live smoke (no/low tokens):** against the running local stack —
      ship a conversation via REST, comment on a task, block-with-question →
      answer → ready, split a task, promote drafts; then attach one MCP
      client session (cockpit grant) and exercise read + create_conversation
      + promote. Record outcome.
- [x] 13. Final: change history complete; milestone verdict recorded; commit.

## Verification

- Unit/integration: `uv run pytest` green throughout; `uv run ruff check .`
  clean; every board mutation lands as exactly one board commit with the
  established message format.
- Milestone test (from 021 M2 intent): a conversation shipped via MCP lands
  as a pending board object; drafts can only enter execution through
  explicit promotion; a split rewires dependencies atomically; the MCP
  surface exposes only its grant set and REST remains the canonical path.

## Loose Ends / Follow-up (post-M2)

Triage state as of M3 close (2026-06-12):

1. **Thread-content injection into session context** — threads are stored
   and served, and `[answer]` re-readies a task, but no session prompt ever
   includes the thread yet. Deferred through M3 (decomposition didn't need
   it); belongs with the first real blocked-question round-trip (M4
   cockpit/weekend narrative).
2. **Event seam carries no transport** — `emit_event` logs only.
   Call-sites in place: `conversation_shipped`, `task_blocked_on_question`.
   M4 puts Telegram behind it and adds the missing gate events
   (drafts-await-promotion, feature→review, task failed).
3. **MCP mounts 307-redirect the bare path** — documented (trailing slash);
   a path-normalizing wrapper would be nicer but isn't worth it yet.
4. **Live-board residue** — `m2-smoke` feature with 3 question-blocked
   branchless tasks; conversation 002 retired with an empty
   `decomposed_into`. Harmless sandbox junk; deletable by hand.
5. **`split` has no inter-child dependencies** — children inherit the
   parent's deps verbatim; ordering between children isn't expressible.
   Extend when a real split needs it.

## Change History

- [2026-06-12] Plan created from 021 M2 scope (+ learnings and loose ends
  from plan 022: rate-limit requeue fix pulled in as todo 10; README staleness
  as todo 11; notifier seam noted as judgment-scope item 8).
- [2026-06-12] Todo 1 recon done. Findings:
  - **Board layer:** `parser.py` uses python-frontmatter; `writer.py` owns
    git commits (`commit_board`/`commit_and_push_board`) with message
    conventions `[board:task|feature|project] <verb> <path>` (API-initiated)
    and `[system:task] ...` (system-initiated); frontmatter update helpers
    preserve bodies. `loader.py` walks
    `projects/<p>/board/features/active/<f>/{_feature.md,tasks/<id>.md}`,
    skipping `*.result.md` — must also skip `*.thread.md` (todo 2).
  - **Concurrency model:** one global `board_lock` (api/deps.py); every API
    request holds it via the `get_board_context` dependency; the supervisor
    tick takes the same lock. In-memory `BoardState` is mutated in place
    after each write; the git watcher full-reloads state whenever HEAD
    moves. Consequence: conversations need an in-memory model + loader
    coverage (the pending queue serves from state); threads can be read
    from disk on demand (no state caching needed). MCP tools must enter
    through the same lock + service layer as REST.
  - **Status reality:** task create already lands `draft` (server-side);
    feature create lands `open` (needs `draft`). No transition validation
    exists anywhere (PUT/PATCH accept any enum value) — todo 6 adds the
    transition table. `TaskStatus` lacks `superseded`; `FeatureStatus`
    lacks `draft`. Dead `Provider` enum in borealis models noted (cleanup
    candidate while touching models.py).
  - **Aurora side:** rate-limited→`queued` mapping at
    `stages/runner.py:81` (`_SESSION_FAILURE_STATUS`) — todo 10 flips it to
    `ready` (Aurora's enum already has READY; cooldown then applies).
    TaskRunner reports via `update_task_status(status, result_content)` —
    `blocked_reason` rides the same PATCH as an optional field (todo 5).
  - **MCP library decision:** official `mcp` SDK (1.27.2) — already in the
    workspace venv (transitive via claude-agent-sdk) and verified to import
    and expose `FastMCP.streamable_http_app()`, mountable inside the
    existing FastAPI/uvicorn process. jlowin `fastmcp` not installed; no
    need for a second dependency tree. Will add explicit `mcp` dep to
    borealis `pyproject.toml`. Direction for grant filtering (firm up in
    todo 8): one FastMCP instance per grant set mounted at `/mcp/{grant}`,
    each registering only its granted tools, with bearer-token check —
    tool-list-per-grant becomes trivially correct.
  - No scope adjustments needed; plan file list matches reality.
- [2026-06-12] Todo 2 done. Judgment calls: conversations/threads folded into
  the existing `parser.py`/`writer.py` (same machinery, files still small)
  instead of a new `board/conversations.py`; thread entry format is
  `## [kind] author — <ISO timestamp>` header + markdown body, parser skips
  any section whose header doesn't parse (lenient for hand edits, regex-anchored
  kinds `question|answer|note`); conversation IDs zero-padded sequential like
  tasks; `blocked_reason` enum gets a third value `infra` now (todo 5 uses it
  for auth/infra blocks) plus `question`/`dependency` per 021; dead `Provider`
  enum removed from borealis models (unreferenced). Loader: conversations
  loaded into `ProjectModel.conversations` (pending queue serves from state);
  `*.thread.md` skipped everywhere companions are skipped. 14 new tests
  (176 total green).
- [2026-06-12] Todo 3 done: `api/conversations.py` (create/list/detail/status
  PATCH with forward-only transition table pending→decomposing→decomposed,
  409 on illegal jumps; `decomposed_into` + companion result file ride the
  status PATCH, same pattern as task results), global
  `GET /api/conversations/pending` in main.py (optional `?project=` filter,
  oldest-first). Notifier seam landed as `service/events.py::emit_event`
  (one function, logs for now; first call-site = conversation shipped).
  Side fix: `push_board` is now a no-op without an `origin` remote,
  matching `sync_remote`'s documented behavior — lets tests (and local-only
  boards) commit for real instead of mocking the push. 6 REST tests
  (181 total green).
- [2026-06-12] Todo 4 done: `api/comments.py` — GET/POST comments on tasks
  (`<id>.thread.md`) and features (`_feature.thread.md`), typed
  `question|answer|note` (422 otherwise), author required, timestamp
  server-side; commit message `[board:comment] <ref> [<kind>] by <author>`.
  Append-only: no edit/delete routes exist. 4 REST tests (185 total green).
- [2026-06-12] Todo 5 done. Status PATCH: optional `blocked_reason` field,
  422 unless status=blocked and value valid; leaving blocked clears the
  reason (frontmatter key removed — `_update_frontmatter` now treats None
  as delete-key); `task_blocked_on_question` flows through the event seam.
  `[answer]` on a question-blocked task → `ready` + reason cleared +
  `ready_at` cleared (fresh cooldown), thread append + flip in **one**
  commit suffixed `(answered → ready)`; `[note]` and answers on
  infra/dependency blocks don't unblock. Aurora: `update_task_status` gains
  `blocked_reason` kwarg; TaskRunner stamps `infra` on every Aurora-side
  block (auth/config/worktree — blocked-on-question only ever originates
  board-side), nothing on other statuses. New fields exposed in task
  GET/list. 5 new tests (190 total green).
- [2026-06-12] Todo 6 done. Judgment calls: the promote verb is the **only**
  exit from draft (PATCH rejects all transitions out of draft — stricter
  than "no illegal jumps", keeps one promotion path); promoting a task in a
  draft feature is 409 (promote the feature first); the transition tables
  also gate the status field on PUT full-updates (otherwise PUT is a gating
  bypass); `superseded` enterable only via split (never via PATCH);
  merged/closed terminal at the API (requeue endpoint owns resurrection).
  Task table mirrors real flows only (queued→in_progress→done/failed/...,
  rate-limit/stale resets, done/failed→ready re-runs) — three existing
  tests used queued→done shorthand and now start tasks `in_progress`.
  Feature create lands `draft`; promote verbs
  `POST .../promote` (task draft→ready, feature draft→open). 6 new tests
  (196 total green).
- [2026-06-12] Todo 7 done: `POST .../tasks/{id}/split`. Judgment calls:
  unsplittable = done/superseded/in_progress (failed and blocked stay
  splittable — that's exactly the oversized-scope escape hatch); children
  of a draft parent land draft, children of a promoted parent land `ready`
  (the parent already passed the human draft gate — a split refines
  approved scope, it doesn't re-enter intake); child entries are
  `{title, body, pipeline?}` only (pipeline defaults to parent's; no
  inter-child deps in M2 — children inherit the parent's deps verbatim);
  failure atomicity = validate-everything-then-write (no partial commit;
  a midway crash leaves an uncommitted working tree, no board history
  damage). Interaction caught: feature→review all-done check now treats
  `superseded` as non-blocking (else a split parent pins its feature
  in_progress forever). Commit format
  `[board:task] split <ref> → <child ids>`. 4 tests (200 total green).
- [2026-06-12] Todo 8 done: `service/mcp.py` on the official `mcp` SDK
  (FastMCP, `stateless_http=True, json_response=True`), **one server per
  grant** mounted at `/mcp/{grant}` — tool-list-per-grant is correct by
  construction, no request-time filtering. Grants per plan: decomposer =
  read + create_feature/create_task; implementer = read + add_comment/
  split_task; reviewer & cockpit = read + add_comment/promote_draft/
  create_conversation (judgment: read tools granted to all four — the
  single-writer principle restricts writes, not reads). Tools call the API
  endpoint functions through a new `deps.acquire_board_context()` (same
  lock, same code path ⇒ identical board commits, verified by test).
  Token auth: `Settings.mcp_tokens` = `grant:token,...`, per-mount ASGI
  bearer guard, unconfigured grants stay open (loopback-only service;
  defense-in-depth per 021). Session managers run inside the app lifespan;
  SDK's DNS-rebinding guard kept (allowed hosts 127.0.0.1/localhost).
  `mcp` added to borealis deps (was already in the lock transitively).
  6 tests (206 total green).
- [2026-06-12] Todo 9 done: CLI thin wrappers — `conversation create/list/
  show` (`--content`/`--content-file`/`--source`), `comment add/list`
  (`--task-id` targets a task thread, omitted = feature thread; kind
  defaults `note`, author defaults `cli`), `feature promote`,
  `task promote`, `task split` (`--tasks-json` inline or `--tasks-file`,
  raw JSON passed through to the API — keeps the CLI thin). 7 tests
  (213 total green).
- [2026-06-12] Todo 10 done: Aurora rate-limit mapping flipped
  `RATE_LIMITED → ready` (stage runner) so the board's ready→queued
  cooldown provides the backoff. Companion board fix: the status PATCH
  clears `ready_at` on every (re)entry to `ready` — without it the stale
  stamp would let the resolver re-queue instantly and the cooldown would
  never apply. Updated 3 Aurora tests, 1 new Borealis test (214 green).
- [2026-06-12] Todo 11 done: README gains "Board objects (Borealis)"
  (conversations, draft lifecycle, threads + blocked_reason, split, MCP
  grant table + token config) and the new CLI rows; Requirements de-staled
  (Claude Code CLI and model pulls dropped; opencode + provider is the
  requirement). `install.sh` still installs/authenticates Claude Code —
  left as-is, installer revalidation remains 022 loose end §6's tail, out
  of M2 scope.
- [2026-06-12] Todo 12 done — **live smoke PASSED, zero model tokens.**
  Borealis+Aurora restarted on M2 code (manual uvicorn, as M1 left them);
  Aurora paused for the duration. Against the live board (`north-test`):
  shipped conversation 001 → appeared in `/api/conversations/pending`;
  feature `m2-smoke` + 2 tasks landed draft; draft gates verified live
  (task-promote-before-feature 409, draft→done 409); promote feature +
  task; note comment; block-with-question → answer comment flipped the
  task to ready (one commit, reason+ready_at cleared); split 001 → 003/004
  (deps inherited, dependent 002 re-pointed to both children, parent
  superseded); conversation walked pending→decomposing→decomposed with
  result file, pending queue drained. Board log: exactly one
  `[board:*]`/`[system:*]` commit per mutation. MCP (cockpit, raw
  JSON-RPC over streamable HTTP): initialize/tools-list (exact grant set),
  get_review read, create_conversation + promote_draft wrote real board
  commits identical to REST's, create_task correctly "Unknown tool".
  One live finding: Starlette mounts 307-redirect the bare `/mcp/{grant}`
  path — README now documents the trailing-slash URL. Aurora resumed;
  m2-smoke's ready tasks will block `infra` when picked (branchless
  feature) — sandbox junk, no spend.
- [2026-06-12] Todo 13 — **M2 MILESTONE VERDICT: PASSED.** All four clauses
  of the Verification section demonstrated live: a conversation shipped via
  MCP landed as a pending board object; drafts could only enter execution
  through explicit promotion (server-enforced, 409s verified); a split
  rewired dependencies atomically in one commit; the MCP surface exposed
  exactly its grant set while REST remained the canonical path (MCP tools
  call the API layer — parity verified by commit messages). Net new:
  conversations + pending queue, comment threads, blocked_reason +
  answer-unblock, draft gating + promote verbs + transition tables, split,
  MCP surface (4 grants), CLI coverage, rate-limit-requeue fix.
  214 tests, ruff clean. Loose ends for later milestones: thread-content
  injection into session context (M3); `task_blocked_on_question` /
  `conversation_shipped` events flow through the one-function seam, real
  transports in M4; install.sh still stale (022 §6); smoke services left
  running (manual uvicorn, logs /tmp/{aurora,borealis}.log); m2-smoke
  feature + 3 ready branchless tasks left on the live board (will block
  `infra` when picked — harmless, deletable).

## Appendix — Kickoff prompt for a fresh context

```
We're continuing North's v3 architecture. Setup context:

1. Read docs/plans/021_session-pipeline-architecture.md — the canonical
   direction doc. Trust it over anything else.
2. Read docs/plans/022_session-executor.md — M1, complete; skim its Change
   History and "Loose Ends" so you know the live-env state and agreed
   simplifications.
3. Read docs/plans/023_board-extensions-mcp.md — the M2 implementation plan.
   Execute it now, working through the todos in order, starting with todo 1
   (read-only recon).

Working rules (same as M1):
- Branch north-v3 (already checked out). Commit per completed todo. Never
  push. Never touch main.
- Update plan 023's todo checkboxes and Change History as you go; run
  "uv run ruff check ." and "uv run pytest" after each todo.
- Small design choices 021 doesn't answer: use your judgment, note it in the
  plan's Change History, keep moving — stop only for hard-to-reverse
  decisions.
- M2 is board-side: REST/MCP testing needs no model tokens. If a model is
  ever needed, prefer local ollama; opencode-go subscription models sparingly
  (smoke only). Do not use local models as session orchestrators (M1
  finding — they can't drive the tool loop).
- Live env from M1 may still be running: aurora :8000, borealis :8001,
  opencode :4096 (manual uvicorn/nohup processes, logs in /tmp/). Board:
  ~/.north/borealis/board (origin = local bare board-remote.git). Sandbox:
  ~/.north/sandbox/north-test (disposable; managed clone at
  ~/.north/aurora/repos/north-test). Restart services freely after Borealis
  code changes.

Cook through as far as you can get.
```
