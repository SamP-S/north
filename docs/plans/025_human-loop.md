# 025 — M4: Human Loop (Notifications, Review Briefs, Cockpit)

Implements milestone M4 of the Build Order in
[021_session-pipeline-architecture.md](021_session-pipeline-architecture.md)
— governing sections: "Review Escalation (resolved)", "The Cockpit
(vocabulary)", and the cockpit-relevant parts of "Conversation Frontend".
M1–M3 are complete (plans 022–024, all passed): the session spine, the
board objects/MCP surface, and the decomposition loop all work live. M4
closes the loop around the human: events reach the phone, features arrive
with a brief, and one persistent conversational session (the cockpit) is
the place where conversations are shipped, drafts promoted, questions
answered, and reviews discussed. Voice is **M5** — everything here is
text-only by design.

## Context

Working agreements (user-confirmed, carried from M1–M3): branch `north-v3`;
commit per completed todo; never push; never touch main; update this plan's
checkboxes + Change History as you go; `uv run ruff check .` +
`uv run pytest` after each todo; small unanswered design choices → judgment
+ note here; stop only for hard-to-reverse decisions. Tokens: opencode-go
sparingly (smoke only); local models are not orchestrator seats. The
cockpit session itself runs on the user's Claude Code subscription — keep
smoke interactions short.

Live env (state at M3 close): manual uvicorn aurora :8000 / borealis :8001,
opencode :4096, logs `/tmp/{aurora,borealis,opencode-serve}.log`; board
`~/.north/borealis/board` (local bare remote); sandbox
`~/.north/sandbox/north-test`; MCP at `http://127.0.0.1:8001/mcp/{grant}/`
(trailing slash). The notifier seam exists
(`borealis/service/events.py::emit_event`, log-only) with two call-sites:
`conversation_shipped`, `task_blocked_on_question`.

**User inputs needed during this plan (not blockers to start):** a Telegram
bot token + chat id (todo 3's live leg; everything else proceeds without
them — the transport degrades to log-only when unconfigured).

## Scope

**Build:**

1. **Notifier behind the seam (Borealis).** `events.py` grows a dispatch:
   `emit_event(kind, **fields)` → format → dedupe/rate-limit → transport.
   Transport interface + two implementations: `log` (default, current
   behavior) and `telegram` (outbound HTTPS `sendMessage`; token + chat id
   from settings; never inbound — 021). Dedupe: identical (kind, key
   fields) within a window collapse to one send; global rate cap so retry
   loops can never storm the phone. Sending must never block or fail a
   board mutation (queue + background sender task, drop-on-overflow with a
   log line).
2. **Complete the gate events.** Existing: conversation shipped, task
   blocked on question. Add: `conversation_decomposed` (drafts await
   promotion — fields: project, conversation, created refs),
   `feature_review` (feature entered review), `task_failed`. All emitted
   Borealis-side where the status change lands (single event source —
   Aurora's reports arrive via the status PATCH).
3. **Service-health notifications, two layers (021):**
   (a) in-process `logging.Handler` forwarding WARNING+ records through the
   notifier with its own dedupe (per logger+message-template, generous
   window) — installed in **both** services; Aurora needs its own transport
   client (small shared-shape duplication is fine; a shared package is
   overkill for two files — judgment, revisit if a third consumer appears).
   (b) systemd `OnFailure=north-notify-failure@%n.service` on all North
   units + a tiny unit running a curl script to the Telegram API (works
   even when the Python process can't self-report). Unit files in
   `systemd/`, wired in `install.sh` copies only — actual systemd adoption
   stays a user action (services currently run manually).
4. **Review briefs.** When a feature enters review, assemble a
   deterministic brief: task list + statuses, accumulated handoff notes
   (from task results), final gate report, `git diff --stat
   base_branch...branch`. Aurora assembles (it has the repo); trigger =
   supervisor notices a feature newly in review (it already polls
   `/api/review`). Stored feature-side as the feature thread entry
   (`[note]` by `aurora/brief`) — the companion-thread pattern from M2,
   no new file kind. The `feature_review` notification carries the
   one-line summary ("feat-x ready: N tasks, +A/−D, gates green").
5. **Refine rule (Borealis).** Creating a task on a feature whose status is
   `review` flips the feature to `in_progress` (one board commit covering
   both writes). Feature returns to review via the existing
   all-tasks-done promotion. Transition table already permits
   review→in_progress.
6. **Cockpit assembly (config + docs, minimal code).** The `north-design`
   session: a `scripts/cockpit.sh` that opens/attaches a tmux session
   running Claude Code in a cockpit workspace directory containing:
   `.mcp.json` (borealis MCP, **cockpit** grant, trailing-slash URL),
   `.claude/settings.json` permission profile (deny Edit/Write/mutating
   Bash; allow read-only repo inspection + the board MCP tools), and a
   `CLAUDE.md` describing the cockpit role (ship condensed conversations
   via `create_conversation`, promote drafts, answer questions via
   comments, review briefings; **never** approve/rollback/reject — those
   are CLI-only by design). Repo read access: the cockpit workspace gets
   read-only paths to the managed clones (additionalDirectories or
   equivalent — confirm mechanism in recon).
7. **README/docs:** notifications (transports, env vars, dedupe), briefs,
   refine rule, cockpit setup + safety asymmetry.

**Out of scope (resist):** voice (M5), ntfy transport (recorded swap
candidate — interface makes it a config change later), memory (M6),
race/vote (M7), stage-index checkpointing, conversation condensing
automation (the cockpit's LLM does it conversationally — no code).

## Files

- New: `borealis/borealis/service/notify.py` (transports + dedupe +
  background sender), `aurora/aurora/service/notify.py` (WARNING+ handler +
  telegram client, mirror), `borealis/tests/test_notify.py`,
  `aurora/tests/test_notify.py`, `aurora/tests/test_review_brief.py`,
  `systemd/north-notify-failure@.service`, `scripts/notify-failure.sh`,
  `scripts/cockpit.sh`, cockpit workspace templates (location judgment —
  likely `cockpit/` in-repo with `.mcp.json`, `settings.json`, `CLAUDE.md`).
- Modified: `borealis/service/events.py` (dispatch through notify),
  `borealis/service/config.py` (+`notify_transport`, `telegram_bot_token`,
  `telegram_chat_id`, dedupe windows), `api/tasks.py` (task_failed,
  feature_review emit + refine rule), `api/conversations.py`
  (conversation_decomposed emit), `aurora/.../supervisor.py` (brief
  trigger), new brief assembly module (`aurora/.../review_brief.py`),
  `aurora/.../config.py`, both services' logging setup, README,
  `install.sh` (unit copies).

## Todos

- [x] 1. **Recon (read-only):** map both services' logging setup (where a
      handler attaches once); confirm how Aurora's supervisor can detect
      "newly in review" cheaply (poll diff vs. event); check Claude Code
      project-config mechanics for the cockpit workspace (.mcp.json shape,
      permission settings keys, read-only additional directories); Telegram
      `sendMessage` API surface. Record findings; adjust scope items if
      reality disagrees.
- [x] 2. Notifier core (Borealis): transport interface, log + telegram
      transports, dedupe/rate-limit, non-blocking background sender +
      tests (fake transport; no network in tests).
- [x] 3. Gate events: `conversation_decomposed`, `feature_review`,
      `task_failed` emits + tests; live Telegram leg **if** token/chat id
      provided (else verified against the log transport and noted).
- [x] 4. WARNING+ forwarding in both services with dedupe + tests; Aurora
      notify mirror.
- [x] 5. systemd `OnFailure=` unit + notify-failure script + install.sh
      wiring (files only; adoption remains manual).
- [x] 6. Review briefs: assembly (statuses, handoff notes, gate tail,
      diffstat), posted as feature-thread note, one-line summary in the
      `feature_review` event; supervisor trigger + tests (tmp repos).
- [x] 7. Refine rule: task-create on review feature → in_progress in one
      commit + tests.
- [x] 8. Cockpit assembly: workspace templates, cockpit.sh, permission
      profile, README section.
- [x] 9. **Live smoke — the weekend narrative, text-only (021 M4 test):**
      from a cockpit session: ship a conversation → decomposition →
      notification (drafts await) → promote from cockpit → execution →
      block a task with a question → notification → answer from cockpit →
      ready → done → feature review notification with brief line → read
      the brief in the cockpit → verdict via CLI (approve). Record outcome,
      notification timeline, and costs.
- [x] 10. Final: change history + loose ends; milestone verdict; commit.

## Loose Ends / Follow-up (post-M4)

Known items deliberately left open at M4 close:

1. **Telegram transport never fired live** — **closed [2026-06-14]**.
   `~/.north/.env` now holds real `TELEGRAM_BOT_TOKEN`/`TELEGRAM_CHAT_ID`
   (bot `@north_notify_bot`, chat id from `getUpdates`). Borealis/Aurora
   restarted with the env loaded; live events confirmed delivered
   (`200 OK` from `api.telegram.org`): `conversation_shipped`,
   `task_blocked_on_question`-style WARNING/ERROR forwarding (Aurora's
   failed decompose attempt — opencode wasn't running, zero token cost),
   and `conversation_decomposed`. Remaining sliver: the systemd
   `OnFailure=` leg (`systemctl --user kill` test) still needs the units
   actually installed — services still run manually (uvicorn/nohup), so
   that part of this loose end stays open until systemd adoption.
2. **Cockpit session not yet run interactively** — the workspace
   (`cockpit/` + `scripts/cockpit.sh`) was validated as config and its
   grant exercised over MCP, but no real `claude` session has started in
   it (subscription frugality). First real use should confirm the
   permission profile bites (Edit denied, `mcp__borealis` allowed,
   additionalDirectories reach the clones).
3. **Thread injection into implement sessions** (023 §1 remainder) — the
   cockpit question round-trip works; sessions still don't see thread
   content mid-task. Defer until a session actually needs an answer
   mid-run rather than a re-queue.
4. **feature_review waits for Aurora** — the notification rides the brief
   note, so a feature moved to review while Aurora is down notifies only
   on Aurora's next tick (accepted: features normally enter review via
   Aurora's own task-done PATCH).
5. **Notifier dedupe state is in-memory** — a restart forgets windows
   (worst case: one duplicate notification after restart) and pending
   queue contents. Harmless at this volume.
6. **Brief trigger polls every tick** — `/api/review` + one comments GET
   per review feature each 5s. Fine at hobby scale; make it event-driven
   only if it ever shows up in profiles.
7. **Sandbox remote quirk fixed env-side** — `receive.denyCurrentBranch
   updateInstead` set on `~/.north/sandbox/north-test`. Real projects
   need a bare/hosted remote; first live approve also closed the archive
   staging bug (fixed in code, todo 9).
8. **install.sh staleness** (022 §6) unchanged beyond the new unit copy —
   still installs/authenticates Claude Code for the retired spine.

## Verification

- `uv run pytest` green throughout; `uv run ruff check .` clean; no test
  ever performs network I/O (fake transports).
- Milestone test = todo 9: the weekend acceptance narrative from 021,
  text-only, demonstrated live.
- Notification discipline: a deliberately induced retry loop produces one
  notification, not a storm (dedupe test + live observation).

## Change History

- [2026-06-12] Plan created from 021 M4 scope + M2/M3 loose ends folded in:
  thread-content injection (023 §1) lands implicitly with the cockpit
  question round-trip (the cockpit reads threads via MCP; injection into
  *implement sessions* stays deferred until a session actually needs
  mid-task answers); event transports (023 §2) are the core of this plan.
- [2026-06-12] Todo 1 recon done (read-only). Findings:
  - **Logging:** Aurora configures root logging once in its lifespan
    (`logctx.configure_logging` — root handler + task-id filter); the
    WARNING+ notify handler attaches there. **Borealis configures nothing**
    — uvicorn's default dictConfig only wires `uvicorn.*` loggers, so
    Borealis app logs (incl. `events.py` INFO lines) reach the stderr
    `lastResort` handler at WARNING+ only. Todo 4 adds a small
    `configure_logging()` to Borealis's lifespan (root handler, mirrors
    Aurora's shape minus task-id), which is also where the notify handler
    attaches once.
  - **Event sites:** `emit_event` seam confirmed (`service/events.py`,
    log-only, called from `api/conversations.py::create_conversation` and
    `api/tasks.py::update_task_status` question-block). Feature→review
    lands in two places: `api/tasks.py` all-tasks-done promotion and
    `api/features.py` PATCH/PUT transitions — emits must cover both.
    `task_failed`: status PATCH with `status=failed`. `conversation_decomposed`:
    `api/conversations.py::update_conversation_status` → `decomposed`
    (decomposed_into rides the same PATCH → created refs available).
  - **Sender shape:** API handlers are sync (threadpool) under one board
    lock; the supervisor is async. The background sender is therefore a
    daemon **thread** + bounded `queue.Queue` (`put_nowait`, drop-on-full
    with a log line) — safe from both contexts, never blocks a board
    mutation, no event-loop coupling.
  - **Brief trigger + event ordering (judgment, reconciles scope §2 vs §4):**
    a feature always enters review via a board write while Aurora is alive
    (task-done PATCH), and the brief follows within one poll. So the
    `feature_review` notification is emitted Borealis-side **when the brief
    note lands** (a `[note]` by author `aurora/brief` on a feature in
    `review`), carrying the brief's one-line summary — single notification,
    rich content, still emitted where the board write lands. Supervisor
    trigger: each tick, brief any `/api/review` feature whose thread lacks
    an `aurora/brief` note (restart-safe, also covers manual PATCHes into
    review; no in-memory seen-set needed).
  - **Result-file shape for briefs:** task results render `## Handoff notes`
    / `## Gate reports` / `## Session manifest` / `## Log`
    (task_runner.py::_render_result) — the brief assembler extracts the
    handoff-notes section + last gate report per task via these headings.
  - **Cockpit mechanics (Claude Code):** `.mcp.json` at workspace root with
    `{"mcpServers": {"borealis": {"type": "http", "url":
    "http://127.0.0.1:8001/mcp/cockpit/"}}}`; `.claude/settings.json`
    `permissions.deny` for Edit/Write/NotebookEdit + mutating Bash,
    `permissions.allow` for read-only Bash + `mcp__borealis__*`;
    `permissions.additionalDirectories` grants *access* (not read-only) to
    the managed clones — read-only is enforced by the tool denies, which is
    acceptable (judgment). Cockpit MCP grant already exists server-side
    (mcp.py GRANTS: read tools + add_comment/promote_draft/
    create_conversation — exactly the 021 curate set; no verdict verbs).
  - **Telegram:** outbound-only `POST
    https://api.telegram.org/bot<token>/sendMessage` with
    `{chat_id, text}`; no webhook (nothing inbound), plain-text messages
    (no parse_mode → no escaping pitfalls).
  - **systemd/install.sh:** units live in `systemd/` with `@@WORKING_DIR@@`
    substitution via `scripts/install.sh` `_install_unit`; the
    `OnFailure=` drop-in + templated notify unit follow the same pattern.
    (install.sh staleness — Claude Code install/auth steps — is 022 §6,
    out of M4 scope beyond the unit copies.)
  - No scope changes needed; file list stands (plus
    `borealis/.../main.py` logging setup).
- [2026-06-12] Todo 2 done: `borealis/service/notify.py` (Transport
  protocol, LogTransport, TelegramTransport, Notifier with producer-side
  dedupe per (kind, key) + global per-minute rate cap + bounded queue +
  daemon sender thread; `drain()` for deterministic tests),
  `events.py::emit_event` dispatches through a lazily-built process-wide
  notifier (`set_notifier` test seam). Judgment calls: a `summary` field on
  an event becomes the human-facing message body (other fields stay the
  dedupe key); rate-cap hits log once per window; transport failures are
  logged, never raised (worker survives); `build_transport` degrades to log
  on missing telegram config or unknown name. Config: `notify_transport`,
  `telegram_bot_token`, `telegram_chat_id`, `notify_dedupe_window_s`,
  `notify_rate_limit_per_min` (+`log_notify_dedupe_window_s` for todo 4).
  13 new tests (248 total green).
- [2026-06-12] Todo 3 done: `conversation_decomposed` emits on the
  decomposed status PATCH (created refs from `decomposed_into`; the
  failed-decompose `decomposing → pending` path emits nothing);
  `task_failed` emits on the failed status PATCH; `feature_review` emits
  when the brief lands — a `[note]` by author `aurora/brief`
  (`comments.py::BRIEF_AUTHOR`) on a feature in `review`, with the note's
  first line as the notification summary (per recon judgment: single
  notification carrying the rich summary, still emitted Borealis-side on a
  board write; brief notes on non-review features or by other authors emit
  nothing). 7 new tests in `test_gate_events.py` (255 total green).
  **Telegram live leg pending**: token + chat id must come from the user —
  not available this run; events verified against the log transport in
  tests, live log-transport observation folds into todo 9's smoke. Wiring
  Telegram later = setting `NOTIFY_TRANSPORT=telegram`,
  `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID` in the env, zero code.
- [2026-06-12] Todo 4 done: `NotifyLogHandler` (WARNING+, dedupe per
  logger+message-*template* so retry loops with varying args collapse;
  own generous window `log_notify_dedupe_window_s=3600`; excludes the
  notify module's own logger so a transport failure can never feed back
  into another send; defensive level guard for direct `handle()` calls).
  Borealis gains `logsetup.py::configure_logging` called from the lifespan
  (recon: uvicorn never configured Borealis's root logger — INFO app logs
  previously only reached stderr via `lastResort` at WARNING+); Aurora's
  `logctx.configure_logging` attaches the handler alongside the existing
  task-id stream handler. Aurora mirror `aurora/service/notify.py`
  (deliberate two-file duplication per plan; mirror config fields added).
  12 new tests (267 total green).
- [2026-06-12] Todo 5 done: `systemd/north-notify-failure@.service`
  (oneshot template; `%i` = failed unit) + `scripts/notify-failure.sh`
  (sources `~/.north/.env`, curl `sendMessage`, falls back to `logger`
  when Telegram is unconfigured or the send fails); `OnFailure=
  north-notify-failure@%n.service` added to all four North units
  (aurora/borealis/ollama/opencode); install.sh installs the template via
  the existing `_install_unit` substitution (repo root as working dir —
  the script runs from the repo). Files only; systemd adoption remains a
  user action (services run manually). `bash -n` clean on both scripts.
- [2026-06-12] Todo 6 done: `aurora/service/review_brief.py` —
  `assemble_brief` builds first-line summary ("feat-x ready: N tasks,
  +A/-D, gates green|red|no gate data") + Tasks/Handoff notes/Final gate
  reports/Diffstat sections; handoff + gate sections extracted from task
  results by the `_render_result` headings (recon), gate verdict = last
  `Gate: PASS|FAIL` per result; diffstat from `git diff --stat
  base...branch` in the managed clone (missing clone → "+?/-?" +
  "(diffstat unavailable)", never fatal). Supervisor:
  `brief_new_reviews` each tick — briefs any review feature whose thread
  lacks an `aurora/brief` note (the note is the marker: restart-safe,
  covers manual review transitions; per-feature failures contained and
  retried next tick). BorealisClient gains `get_feature_comments` /
  `add_feature_comment`. 7 new tests (274 total green).
- [2026-06-12] Todo 7 done: refine rule in `tasks.py::create_task` —
  creating a task on a feature in `review` flips the feature to
  `in_progress` in the same board commit (suffix "(review →
  in_progress)"); the existing all-tasks-done promotion returns it to
  review (covered by test: refine → promote → execute → done → review
  again; the refinement task lands draft like any created task, so the
  "accept-but" motion is create + promote from the cockpit). 3 new tests
  (277 total green).
- [2026-06-12] Todo 8 done: `cockpit/` workspace in-repo (judgment per
  plan) — `.mcp.json` (cockpit grant, trailing slash), `.claude/
  settings.json` (deny Edit/Write/NotebookEdit + mutating Bash incl.
  `git commit/push/checkout/...` and the North CLIs; allow Read/Grep/Glob +
  read-only git/Bash + `mcp__borealis`; `additionalDirectories:
  ~/.north/aurora/repos` — access is not read-only per se, the tool denies
  enforce read-only; `enableAllProjectMcpServers: true`), `CLAUDE.md`
  (role: condense→approve→ship, curate/promote, answer questions, brief
  walks; never verdict verbs, CLI-only). `scripts/cockpit.sh`: create-or-
  attach tmux session `north-design` running `claude` in the workspace
  (switch-client when already inside tmux). README "Human loop" section:
  notification env vars/dedupe, two health layers, briefs, refine rule,
  cockpit + safety asymmetry, ntfy swap note.
- [2026-06-12] Todo 9 — **live smoke PASSED**; one real bug found and
  fixed (that's what live smokes are for). Judgment: the cockpit was
  exercised through its MCP grant directly (curl JSON-RPC against
  `/mcp/cockpit/`) rather than a paid Claude Code session — the grant
  surface and flow are what M4 built; a subscription session adds only
  conversation. Narrative, all on the log transport:
  - On the **first startup** of M4 Aurora, the two M3-leftover review
    features (`farewell-module`, `hello-feature`) were briefed exactly
    once each with real diffstats (+45/-0, +14/-0, gates green) —
    `feature_review` fired through the new path immediately; subsequent
    ticks skipped them (brief-note marker works).
  - 17:38:12 ship via cockpit `create_conversation` → notification.
    17:38:46 `conversation_decomposed` (decompose: 30.9s, 12.4k tok,
    $0.00115; created `version-stamp` + 2 drafts, clean handoff).
    Promote feature + 2 tasks via cockpit `promote_draft`. 17:39:28 task
    002 question-blocked (simulated session-side PATCH + thread question)
    → notification; cockpit `add_comment [answer]` → ready instantly.
    Both tasks executed through the M1 spine (4 sessions, $0.0033).
    17:41:11 `feature_review` — "version-stamp ready: 2 tasks, +3/-0,
    gates green"; brief read back via cockpit `get_comments`. Total spend
    ≈ $0.0045.
  - **Verdict via CLI**: first `aurora approve` 500'd — two findings:
    (a) env: the sandbox "remote" is a non-bare checkout refusing pushes;
    fixed with `receive.denyCurrentBranch updateInstead` (approve had
    never run live against the sandbox — M1–M3 features still sat in
    review). (b) **real bug**: feature archival staged the `archived/`
    adds but not the `active/` deletions, leaving the board repo
    permanently dirty — every remote sync (`pull --rebase`) then failed.
    Fixed in `features.py` (status PATCH passes `removed=[active_dir]`;
    `requeue` the mirror case) + 2 regression tests in
    `test_archive_clean.py` (279 total green); live board cleaned by a
    sysadmin commit. Approve retry: merged, archived, sandbox main got
    VERSION=0.1.0.
  - **Live dedupe proof, organic**: the dirty-board sync error logged 13+
    times (one per 5s poll) but produced exactly **one** notification —
    the "retry loop ≠ storm" requirement observed live, and the WARNING+
    forwarding layer is what surfaced the bug in the first place.
  - Telegram leg still pending user credentials (log transport verified
    end-to-end; swap is env-only).
- [2026-06-12] Todo 10 closeout: Loose Ends register added (8 items);
  **milestone verdict: PASSED** — the 021 M4 test (weekend narrative,
  text-only) ran live end to end: ship → decompose → notification →
  promote (cockpit grant) → execute → question-block → notification →
  answer (cockpit grant) → done → review notification with brief line →
  brief read via cockpit → CLI approve → merged + archived. Learnings fed
  back into 021's change history; Build Order M4 marked done/passed.

## Appendix — Kickoff prompt for a fresh context

```
We're continuing North's v3 architecture. Setup context:

1. Read docs/plans/021_session-pipeline-architecture.md — the canonical
   direction doc. Trust it over anything else.
2. Skim the Change History + Loose Ends of docs/plans/022, 023, 024
   (M1–M3, all complete/passed) for live-env state and agreed
   simplifications.
3. Read docs/plans/025_human-loop.md — the M4 implementation plan. Execute
   it now, working through the todos in order, starting with todo 1.

Working rules (same as M1–M3):
- Branch north-v3 (already checked out). Commit per completed todo. Never
  push. Never touch main.
- Update plan 025's todo checkboxes and Change History as you go; run
  "uv run ruff check ." and "uv run pytest" after each todo.
- Small design choices 021 doesn't answer: use your judgment, note it in
  the plan's Change History, keep moving — stop only for hard-to-reverse
  decisions.
- Tokens: opencode-go sparingly (smoke only). Local models are not
  orchestrator seats. Tests must never hit the network (fake transports).
- Telegram bot token + chat id must come from the user — ask when you
  reach todo 3's live leg; build against the log transport meanwhile.
- Live env: aurora :8000, borealis :8001, opencode :4096 (manual
  uvicorn/nohup, logs /tmp/). Board ~/.north/borealis/board (local bare
  remote). Sandbox ~/.north/sandbox/north-test; managed clone
  ~/.north/aurora/repos/north-test. MCP: http://127.0.0.1:8001/mcp/{grant}/
  (trailing slash). Restart services freely after code changes.

Cook through as far as you can get.
```
