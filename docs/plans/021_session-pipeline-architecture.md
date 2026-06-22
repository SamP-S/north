# 021 — Target Architecture: Session Pipelines

**Status: living direction document — under active discussion, expect edits.**
This is not an implementation plan. It captures the agreed target architecture and the
reasoning behind it, so future implementation plans have something to steer by.

## Context & Motivation

North exists to support a multi-month side project (CAM robot software) with this
workflow:

1. Conversational design sessions (voice/text) at high–medium level → condensed,
   well-structured markdown design documents.
2. An autonomous workflow decomposes design docs into discrete features and tasks.
3. A kanban board (Borealis) tracks what needs doing and the correct order.
4. Tasks flow through user-defined, retunable pipelines specifying agents, roles,
   access levels, tools, models, and providers (local or cloud).
5. Each task produces commits → auditable history.
6. When all tasks in a feature are done, it escalates to the human for review:
   accept/merge, refine manually, add refinement tasks, rollback (project + board
   state), or reject entirely.
7. Designed to leverage slow/weak local models — speed is not a requirement.

The original implementation (YAML pipeline + hand-rolled step state machine +
confidence-frontmatter routing) is being superseded by the architecture below.
Note: langgraph was never actually a dependency — it appears only in a docs example.
The production engine is `aurora/service/pipeline/executor.py`, which this direction
eventually retires.

## The Core Decision

**For linear flows, intra-task orchestration is delegated to the agent runtime
(opencode); North keeps the deterministic outer loop.**

The trade-off, named explicitly:

- Custom pipeline engine = *structure substitutes for intelligence*. Weak models do
  one narrow role; deterministic routing does the rest.
- Prompt orchestration = *intelligence substitutes for structure*. The orchestrator
  seat needs a capable model; it delegates mechanical subtasks to cheap local
  subagents (opencode agent definitions carry per-agent model/provider/tools/
  permissions).

We accept this trade. It reshapes the local-model goal into: capable orchestrator,
local subagents — and removes Aurora's most fragile code (artifact frontmatter
parsing, confidence routing, retry machinery).

## The Boundary

**North keeps (deterministic "factory floor"):**
- Borealis as-is: git-backed board, dependency-ordered queue, resolver, cooldowns.
- The outer loop: design intake → decomposition trigger → pop task → run sessions →
  commit → report → escalate feature for human review.
- Worktree/branch lifecycle, approve / rollback / reject.
- **Done-gates**: "done" is decided by deterministic checks (build/lint/test, diff
  non-empty), never by agent self-report. Agent self-assessment is advisory.
- Session-level failure handling (timeout, rate-limit, auth) — at session
  granularity, not step granularity.
- Per-stage commits (harness-side, never trusted to agents).

**The runtime owns (promptware interior):**
- Everything inside a session: which subagent to call when, retries, context
  management, tool loops, delegation.

Rule of thumb: everything inside a session can be vibes; the factory floor is code.

## The Model

### Session profile
A versioned markdown file (frontmatter + prompt): orchestration prompt, agent roster
it may delegate to, model/provider, permission set, MCP server grants, and what the
handoff note should cover. Current agent definitions evolve into these.

### Stage
One invocation pattern of profiles. Stage *types* are a small fixed set implemented
in harness Python — pipeline files only choose and parameterize them (no routing
logic in config, ever — this is the anti-DSL rule):

- `run(profile)` — single session.
- `race(profiles | profile×n)` — N parallel candidate sessions in isolated
  worktrees/branches.
- `vote(judges)` — M judge sessions over candidates; harness tallies.
- `gate(checks)` — deterministic commands; exit code is the only contract.

Stage entries take **profile lists, not profile + count**; heterogeneous entries
supported via inline overrides on a base profile
(`{profile: judge, model: ollama/mistral:7b}` → derived name `judge@mistral:7b`
for audit attribution). Homogeneous count is shorthand. Vote tallying starts with
equal votes + odd judge count; per-judge verdicts recorded so disagreement patterns
are visible; weights only if data justifies them later.

### Pipeline
An ordered stage list with per-stage failure policy (`retry | fail-task |
escalate`). Purely linear and declarative, e.g.:

```yaml
name: experimental-cam
stages:
  - race: { candidates: [impl-sonnet, impl-qwen, impl-mistral] }
  - gate: { checks: [build, test] }
  - vote: { judges: [judge-mistral, judge-big-context, judge-sonnet] }
  - run:  { profile: polish }
  - gate: { checks: [test, lint] }
```

A single-`run` pipeline is the degenerate (and common) case. Switching a task to an
experimental pipeline is a one-line change to its `pipeline` field on the board.

### Session → session handoff (three channels)
1. **Code state — via git.** Commits in the worktree; the next session inspects
   `git log`/`git diff`/tests itself (it has tools; nothing is inlined into prompts).
2. **Handoff note** — short markdown the session ends with (Summary / Decisions /
   Concerns / Suggested status). Captured from the final message. **Advisory, never
   parsed for routing** — malformed notes from weak models cost nothing. Appended to
   the task's result file in Borealis (narrative audit trail).
3. **Objective evidence — harness-produced.** Gate output tails, diff stats, commit
   lists. Deterministic; often the most useful input to a fix session.

The harness threads a context (task body, feature description, accumulated notes,
latest gate report) and renders each stage's opening prompt from it.

### Race isolation
One ephemeral worktree + branch per candidate (`feat-x/cand-N` from the same base
commit). Gates run per-candidate *before* judges (objective elimination first —
free of model bias and often decisive). Selection = merge winning branch into the
feature branch; losers deleted (optionally tagged under `refs/north/experiments/...`
for audit). Candidates are branches; selection is a merge; audit is refs. Local-GPU
parallelism really serializes through Ollama — acceptable, speed is not a goal.

### Checkpointing (free)
Stage boundaries are natural resume points: branch state is in git, notes are in the
task result; the harness persists only the stage index. A rate-limited task resumes
at its stage boundary instead of restarting. (This was the only genuine argument for
adopting langgraph; the session model gets it without a framework.)

### Gates are language-agnostic
- Pipelines name **abstract checks** (`test`, `lint`, `build`); the project repo
  defines them in a versioned manifest (e.g. `.north/checks.yaml`):
  `test: cargo test` / `uv run pytest` / `ctest` per project.
- The harness interprets **exit codes only**. Detailed output needs no machine
  parsing — its consumer is the next session's LLM, which reads raw compiler/test
  output natively. Harness records pass/fail + output tail.
- Missing-check policy per gate: `required` vs `if-present`.
- Decomposition should emit "create checks manifest + test skeleton" as one of the
  first tasks of a new project.
- **Known reserved gap:** execution environments (toolchains on host today;
  mise/devcontainer/nix per project later — both reproducibility and containment).

## Named Principles

These resolved multiple independent questions; future design questions should be
tested against them first:

1. **Single-writer board mutation.** Board writes funnel through one channel per
   task. Harness commits, propose→apply for multi-candidate stages, graph surgery
   as server-side verbs. Agents in race stages never hold board-write tools.
2. **Source of truth vs. derived index.** The repo holds knowledge (design docs,
   conventions); the board holds work; vector stores and MCP surfaces are derived/
   adapters — wipeable, rebuildable, never canonical.

## Board Extensions (Borealis)

**Status: built (M2, plan 023; M3 additions, plan 024).** Decisions made in
implementation that this section didn't specify:

- Status moves are gated by **server-side transition tables** on both PATCH
  and PUT; the promote verb is the *only* exit from draft, and a task cannot
  be promoted while its feature is draft. Entering `ready` always clears
  `ready_at` (fresh cooldown). `superseded` is enterable only via split and
  counts as "done" for feature→review promotion.
- `blocked_reason` gained a third value **`infra`** (auth/config/worktree
  failures stamped by Aurora); `question` only ever originates board-side.
- Conversations are audit objects: no API delete, forward-only status except
  the `decomposing → pending` retry path; failed decompose attempts
  overwrite the companion result (forensics live in transcripts).
- Created items carry **`decomposed_from`**; authoritative
  `decomposed_into` is computed by a harness-side board diff, never from
  agent self-report.

- **`split` as a first-class verb.** `POST .../tasks/{id}/split` takes replacement
  tasks; Borealis relinks `depends_on` atomically in one board commit (children
  inherit parent's deps; dependents of parent re-pointed to children; parent gets
  `superseded` status — kept for audit, showing original scope vs. outcome). Agents
  never edit dependency frontmatter across files.
- **Comment threads per task.** Companion file (e.g. `001.thread.md`, sibling to the
  existing `001.result.md`), append-only typed entries
  (`[question]/[answer]/[note]`, attributed, timestamped). API:
  `GET/POST .../tasks/{id}/comments`. Serves: blocked-task questions, review
  remarks, agent notes. Task `body` stays purely the definition.
- **`blocked_reason` frontmatter** distinguishes blocked-on-human(question) from
  blocked-on-dependency. Answering a question flips the task back to `ready`; the
  thread is injected into the next session's context.
- **Scope escape hatch.** A session that discovers oversized scope stops and
  proposes a split (via handoff note in race stages, via the split verb when it
  holds the grant) rather than scope-creeping inside one commit window.

## MCP Strategy

**Status: Borealis MCP surface built (M2); per-profile grants built (M3).**
Implemented shape: one FastMCP server per grant set
(decomposer/implementer/reviewer/cockpit) mounted at `/mcp/{grant}/`
(trailing slash required) inside the Borealis process — tool-list-per-grant
is structural, not request-time filtering; tools call the REST API layer
under the same lock (REST-canonical held with zero duplicated write logic).
Optional per-grant bearer tokens via `MCP_TOKENS`. On the client side,
opencode's message POST accepts a `tools: {pattern: bool}` map — profiles
carry their grants in frontmatter (decompose: `"borealis_*": true`) while
the opencode config disables `borealis_*` globally, so no other seat ever
sees board tools.

- **Per-profile MCP grants** are part of a profile's capability set, same axis as
  permissions and model (opencode speaks MCP natively). E.g. repo-mapping agent
  gets the memory/RAG MCP; implementer doesn't.
- **Memory, two corpora:**
  - *"What is"* (design state, conventions) → curated markdown in the project repo.
    Auditable, versioned, human-fixable. No RAG for code navigation — agentic
    search (grep/read) is the proven approach.
  - *"What happened"* (task results, handoff notes, verdicts accumulating over
    months) → chromadb, indexed harness-side on task completion, exposed
    read-only via memory MCP. Derived index — wipe/rebuild anytime (which also
    neutralizes rollback-vs-memory questions).
  - **Chroma setup (concrete)**: one `chroma run` server for all of North
    (systemd user unit, loopback-only; server mode because indexer and memory
    MCP are separate processes). One **collection per project**. Local default
    embedding model (bundled MiniLM ONNX, CPU); Ollama embedding model is a
    config swap if quality disappoints. Aurora indexes at task completion
    (handoff notes, gate summaries, verdicts, conversation summaries) with
    metadata `{project, feature, task_id, stage, profile, kind, date}` and
    deterministic IDs (idempotent re-index). Memory MCP exposes one read-only
    tool: `search_history(query, filters) → top-k snippets + task_id pointers`
    (fuzzy recall finds the trailhead; board links + session manifest walk the
    trail). `rebuild-index` replays board git history from scratch.
  - **No agent-self-maintained memory plugins** (opencode-mem, agent-memory,
    Hindsight, etc.) in execution or cockpit configs: North's memory is
    harness-written and source-derived by design — agents hold read-only
    recall, so nothing hallucinated mid-session can pollute long-term memory.
    (opencode core has no built-in memory; only rules + session storage, so no
    inherent conflict exists.)
- **Borealis as MCP server, alongside REST.** Thin adapter (e.g. FastMCP mounted
  next to FastAPI) exposing a curated subset with per-profile tool filtering
  (decomposer: create/split; implementer: comment + split-propose; reviewer:
  read-only). Sleeper benefit: any MCP-capable chat/voice client becomes the
  conversational board frontend — the stage-1 workflow ("what's in review?",
  "create a feature for adaptive feed rates") nearly for free.
  **Rule: REST stays canonical; MCP is a surface, not the spine.** Aurora's loop
  keeps speaking HTTP.
- Domain MCPs later: LSP/symbol navigation (C++/Rust CAM code), library docs;
  potentially the CAM simulator as a tool and/or gate.

## Conversation Frontend (resolved direction)

Constraints (user): conversational voice preferred over dictation-style STT→TTS;
Borealis is **never publicly exposed**; phone reaches the home network via the
user's own VPN; the conversing agent needs **read-only repo access** to check
implementation details.

Consequence: the Claude-app-with-remote-connector path is ruled out — claude.ai
connectors are dialed from Anthropic's servers (phone VPN doesn't help), which
would require public exposure.

**Target shape — conversation host on the North machine:**
- The design-conversation client is a Claude Code / opencode session running on
  the server where North and the repos live, inside tmux (desk and phone attach
  to the same session).
- **Design-conversation permission profile**: deny repo mutation (Edit/Write/
  mutating Bash), allow reads/grep/git inspection; **full board read** (queue,
  features, tasks, review list — the session doubles as the conversational
  cockpit) plus the few board write verbs needed to ship and curate
  (`create_conversation`, comment, promote, draft curation). Broad board
  mutation withheld per the single-writer principle. Expressed as permissions,
  not plan-mode approximation.
- Borealis MCP stays loopback (VPN-reachable at most). Token auth on the MCP
  surface is defense-in-depth, not an exposure enabler.
- **Voice layer (concrete, determined)**: the North host is a **headless server**;
  all devices (desk machine, phone) are clients. Stack, as systemd user units
  alongside aurora/borealis/ollama/opencode:
  - `whisper.service` — whisper.cpp STT server (OpenAI-compatible API,
    VoiceMode-managed install).
  - `kokoro.service` — Kokoro-FastAPI TTS (OpenAI-compatible API).
  - `livekit.service` — self-hosted LiveKit server (single binary), LAN/VPN only.
  - Voice web frontend joining the LiveKit room (mic + speakers in the browser).
  - **VoiceMode MCP** (`voice-mode`) attached to the `north-design` Claude Code
    session — agent-driven `converse` turn-taking; auto-prefers the local
    whisper/kokoro endpoints. Accepted trade-off vs realtime APIs: ~1–2s turn
    latency, no barge-in.
  - **Audio never travels over SSH; text never needs the browser.** Desk and
    phone are the same mechanism: browser tab → LiveKit for audio, SSH + tmux
    attach for text — same session, so a walk conversation continues at the desk.
  - TLS required on the web frontend even VPN-only (browsers need a secure
    context for mic): private CA trusted on devices, or DNS-01 Let's Encrypt cert
    on a domain resolving to the private IP. No public exposure either way.
  - VNC rejected (desktop baggage, poor audio); Claude-app connectors rejected
    (require public exposure); native Claude Code dictation rejected (local-mic
    only, not conversational).
  - Claude Code hosts the *conversation*; opencode remains the *execution*
    runtime — independent concerns.

**Sequencing:** text sessions work today (SSH+tmux). The one experimental leg is
self-hosted LiveKit + web frontend (VoiceMode's local-LiveKit guide is still
"coming soon"; community setups confirm on-LAN operation).

The decomposition flow below is unchanged by any of this — the MCP boundary was
chosen precisely so the frontend could vary.

## The Cockpit (vocabulary)

**The cockpit** is the single persistent conversational session on the North
host — `north-design` in tmux, Claude Code with the design permission profile
(repo read-only; board read; ship/curate verbs) and VoiceMode attached. It is
the human's sole interface to the system, by voice or text, from any device:
design conversations, shipping conversations to the board, promoting drafts,
answering blocked-task questions, and review briefings all happen here.

Safety asymmetry (deliberate): the cockpit **understands and curates, never
fires heavy weapons**. Verdict verbs (approve / rollback / reject) are excluded
from its grants — it may recommend a verdict; executing one is a separate,
deliberate act via SSH + CLI.

## Review Escalation (resolved)

**Status: built (M4, plan 025).** Implementation decisions this section
didn't specify: the `feature_review` notification is emitted when the
*brief note lands* on the feature thread (a `[note]` by `aurora/brief` on
a feature in `review`) — one notification carrying the brief's first
line, still emitted Borealis-side on a board write; the brief note itself
is the supervisor's "already briefed" marker (restart-safe, covers manual
review transitions). WARNING+ forwarding dedupes per logger+message
*template* (varying args collapse). Notification sending is a bounded
queue + daemon thread — it can never block or fail a board mutation.

1. **Notifications — Borealis emits events; transports are config.** A notifier
   interface receives human-gate events: conversation decomposed (drafts await
   promotion), task blocked on question, feature → review, task failed. Plus
   **service health**, two layers: (a) in-process logging handler forwarding
   WARNING+ through the notifier with dedup/rate-limiting (retry loops collapse
   to one notification, never a storm); (b) systemd `OnFailure=` hooks on all
   North units for hard crashes the process can't self-report.
   **First transport: Telegram bot** (user decision) — zero hosting, delivery
   independent of phone-VPN state (server makes outbound HTTPS only; nothing
   exposed inbound), native Android push; accepted trade-off: notification
   content transits Telegram's cloud. **ntfy (self-hosted)** is the recorded
   candidate to swap to after the user evaluates it (fully in-VPN privacy, one
   tiny unit, requires always-on phone VPN for delivery). The notifier
   interface makes the transport a config swap, not a rework.
2. **Review briefing.** When a feature reaches review, the harness assembles a
   deterministic brief (task list + statuses, accumulated handoff notes, final
   gate report, diffstat) stored feature-side. The notification carries the
   one-line summary ("feat-x ready: 4 tasks, +812/−147, gates green").
3. **Conversational review in the cockpit.** Board read + repo read suffice:
   "brief me on feat-x", walk `git diff main...feat-x`, read specific files,
   read reviewer concerns aloud — full review-by-voice while away. Deep manual
   review remains classic: fetch the branch on the desk machine, own tools.
   PR-for-viewing integration: declined for now.
4. **Verdicts via CLI over SSH** (approve / rollback / reject) — human-executed,
   never cockpit-executed (accidental-trigger concern). Existing
   approve/rollback/reject machinery unchanged.
5. **Feature-level comment thread** (`_feature.thread.md`, same companion
   pattern as tasks): review remarks, verdict rationale, briefs.
6. **Refine rule:** adding a task to a feature in `review` automatically reverts
   it to `in_progress`; the feature returns to review when tasks complete again.
   "Accept-but" is one cockpit motion: add refinement tasks, promote, done.

**Acceptance narrative** (the test for this section): away for a weekend —
describe changes by voice → approve the condensed conversation → autonomous
decompose/execute → review notification → conversational brief + selective code
reading → SSH approve → merged to main, feature archived.

## Conversation Intake & Decomposition (resolved)

**Status: built and live-proven (M3, plan 024) — except condensing/shipping
from the cockpit (M4) and racing (M7).** Implementation decisions beyond
this section: the decompose worktree is *detached* at `base_branch` (the
managed clone holds the branch checked out); docs-only violations discard
the **whole** repo diff (board writes stand); every non-OK session outcome
returns the conversation to `pending` (no blocked state for conversations)
with an in-memory supervisor backoff + startup recovery for stranded
`decomposing` ones; decompose has its own timeout (1800s — the seat
explores before writing). Observed model behavior: the decomposer may
task-out the docs distillation instead of writing `docs/` directly —
visible via the guard log, acceptable for now.

Key recategorization (user): condensed conversation outputs are **not design
docs** — they describe *work wanted* ("task bundles"). The board holds work, so
conversations are first-class Borealis objects. Decomposition is *planning* work
— Borealis's domain — not a project task; **pending conversations are themselves
the decomposition queue**. (The earlier "permanent main board" idea is dropped;
it existed only to house decompose tasks, which no longer exist.)

1. **Condense in the client.** The cockpit session condenses the conversation;
   the human approves before shipping. Raw transcripts never ship.
   (First human gate — before autonomous spend.)
2. **Ship = `create_conversation(title, content)`** via Borealis MCP. Stored as
   `projects/<name>/conversations/<id>.md` — frontmatter + body, mirroring the
   task/feature file pattern (same parser/writer machinery, git-audited).
   Frontmatter state: `status: pending | decomposing | decomposed`,
   `created_at`, `source: voice|text`, and on completion `decomposed_into:`
   (features/tasks created). A companion result file holds the session's
   handoff note.
3. **Two queues, decomposition first.** Borealis serves the pending-conversation
   queue alongside the eligible-task queue. Aurora's supervisor polls the
   decomposition queue first; only when it is empty does it take project tasks.
4. **Grounded decomposition.** The decompose run is an Aurora session in a
   `base_branch` worktree (code, `docs/design/`, `AGENTS.md` readable) with
   board-read MCP (link onto existing features, avoid duplicates, set realistic
   task- and feature-level `depends_on`). It reads the conversation from the
   board; status updates are conversation-shaped, not task-shaped.
5. **Outputs:**
   - Features/tasks created via MCP create verbs, landing as **`draft`** —
     **server-enforced**, not prompt-enforced. Created items carry
     `decomposed_from: <conversation id>`.
   - **Durable-decision distillation:** anything in the conversation that
     deserves to outlive its tasks (architectural decisions, conventions) is
     promoted by the decomposer into repo knowledge (`docs/design/`,
     `AGENTS.md`) and committed to `base_branch` under a **docs-only diff
     guard** (deterministic: the session cannot land code on main). Intake →
     board; knowledge → repo.
   - Task bodies are self-sufficient or reference repo docs — implementation
     sessions never need to fetch old conversations.
   - Conversation frontmatter updated: `status: decomposed`, `decomposed_into`.
6. **Always draft.** Human reviews the breakdown in the cockpit and promotes
   `draft → ready` (second human gate — before execution spend). Bad
   decomposition is deletable drafts, not a polluted queue. Per-project
   `auto_ready` is a possible later extension (same shape as `auto_merge`).
7. **Decomposer toolset:** board read + create verbs only (no delete, no status
   changes, no editing existing items). Curation of existing drafts (drop, edit,
   relink) is a cockpit/human activity — the decomposer is a purely additive
   writer (single-writer principle).
8. **Racing decomposition: deferred** until the basic loop is proven. Future
   shape recorded: candidates emit *plans* (text, no board writes), judges vote,
   a small dedicated **apply session** transcribes the winning plan via the
   create verbs (agents do content, harness does control; draft gate bounds the
   damage of a bad apply).
9. **Iteration.** Amending conversations flow through the same path; grounding
   (board-read + repo docs) prevents duplication and links new features onto
   existing ones.

## Known Gaps / Connective Tissue (not yet designed in detail)

- **Project memory discipline**: durable decisions must live in the project repo
  (`docs/design/`, `AGENTS.md`) — the decomposer distills them there (see
  Conversation Intake §5); profiles instruct agents to consult them. Without
  this, quality degrades silently as the project grows.
- **Cross-feature drift**: when a feature merges, in-flight features need a
  "rebase + re-run gates" policy; rebase conflicts become tasks (session-attempted,
  then escalated).
- **Telemetry as the tuning instrument**: per-stage duration, tokens/cost, gate
  pass rates, vote agreement, race win patterns — recorded in task results from day
  one. Pipeline retuning should be data-driven ("does the 5-way race earn its 5×
  cost?"), not vibes.
- **Budget control**: per-task cost caps, especially around race stages.
- **Execution environments**: per-project toolchain definition + containment
  (reserved, see Gates).

(Review escalation: resolved — see its section. Only implementation detail
remains: notification dedup/rate-limit tuning.)

## Migration Consequences (direction-level)

Aurora narrows from "pipeline engine + runtime adapters" to **task lifecycle daemon
+ session launcher**:

- Eventually retired: `pipeline/executor.py` + loader (confidence routing),
  artifact frontmatter parsing, `execution/cloud.py` + `execution/local.py` and the
  legacy runtime (opencode has an Ollama provider — one execution path, per-step
  provider/model), prompt-building layer.
- Grows: `runtime/opencode.py` from "send one message" to "run a session profile to
  completion"; worktree manager gains candidate-branch lifecycle; stage-type
  implementations; checks-manifest resolution; telemetry capture.
- Agent definitions migrate toward opencode-native agent format; North profiles
  reference them.
- Borealis: untouched in spirit; gains split verb, comment threads,
  `blocked_reason`, `superseded` status, MCP surface. The `auto_merge` flag and
  review lifecycle already built remain.

## Resolved Small Questions

- **Losing race candidates**: worktrees deleted always; text evidence (handoff
  notes, gate reports, per-judge verdicts) kept in the task result always;
  commits preserved under `refs/north/experiments/<feature>/<task>/<run-id>/
  <candidate>` — run-id (timestamp) prevents collisions on re-runs (re-runs are
  a designed path: rollback re-queues). Race setup deletes stray `cand-*`
  branches first (idempotent). Refs live in the local managed clone only, never
  pushed; pruned after ~90 days (configurable) so gc reclaims objects. Value:
  judge auditability, model-capability forensics (gate-failed losers included),
  cheap resurrection (`git branch rescue refs/north/experiments/...`).
- **Squash-on-approve: no.** `--no-ff` merge commits (already used by approve)
  make `git log --first-parent main` the clean one-line-per-feature spine view,
  while full stage-granular detail stays reachable for blame/bisect
  (`git bisect --first-parent` for feature-granularity). Squash would destroy
  the forensic view to obtain a view we already have. Document the idiom in the
  README.

- **Handoff-note convention**: profiles *request* `## Summary / ## Decisions /
  ## Concerns / ## Suggested status`; nothing parses it (deviation is free —
  convention as courtesy for human scanning and judge comparison, not contract).
- **Session transcripts**: three tiers. Board task result holds handoff notes,
  gate reports, telemetry summary, and an explicit **session manifest** (stage,
  profile, session id, transcript path, tokens, duration) → deterministic
  two-hop investigation. Full transcripts exported at session end to
  `~/.north/aurora/transcripts/<project>/<task>/<session-id>.json` (don't rely
  on opencode's internal storage), retention configurable. Chroma "what
  happened" index is the later fuzzy-recall layer — a derived index is never
  the only pointer to anything.

## Build Order (roadmap)

Detailed implementation plans are written **just-in-time**, one numbered plan
per milestone (022 = M1, 023 = M2, …), each following the standard plan
structure. Ordering principles: every milestone leaves a working system; the
riskiest bet is proven first; deletions happen early; empty-at-birth components
wait until there is something to fill them.

- **M1 — Session executor (the spine swap)** → plan 022, **done/passed**.
  Session profiles,
  `run_session` (opencode session to completion, transcript export), stage
  runner with `run` + `gate` only, checks-manifest resolution, harness commits
  at stage boundaries, telemetry + session manifest in results. **Retires** the
  pipeline engine (executor/loader), confidence/artifact parsing, cloud/local
  executors, legacy runtime. Test: real task runs `implement → gate → review →
  gate` end to end with commits and linked transcript.
- **M2 — Board extensions + MCP surface** → plan 023, **done/passed**.
  Conversations (state machine + pending queue), comment threads,
  `blocked_reason`, server-enforced draft, split verb, Borealis MCP server
  (curated verbs, per-profile filtering) alongside canonical REST.
- **M3 — Decomposition loop** → plan 024, **done/passed**. Two-queue
  supervisor (conversations first), decompose sessions in `base_branch`
  worktrees, docs-only guard, `decomposed_into` bookkeeping, decompose
  profile (budget: prompt iteration, not just code). Test: conversation →
  drafts → promote → execute → review.
- **M4 — Human loop** → plan 025, **done/passed**. Notifier interface +
  Telegram, gate events, WARNING+ forwarding with dedupe, systemd
  `OnFailure=`, review briefs, refine rule, cockpit assembly (profile +
  tmux + MCP wiring). Test: **weekend narrative, text-only** — ran live.
  (Telegram transport built but unfired pending bot credentials; log
  transport proven.)
- **M5 — Voice — DEFERRED.** Explored both self-hosted (whisper/kokoro/
  LiveKit) and hosted (VoiceMode Connect) audio routing; neither reached a
  working state within scope. Revisit after M1–M7 complete (retry Connect or
  design an alternative routing). See docs/plans/026.
- **M6 — Memory.** Chroma unit, completion-time indexer, `search_history` MCP,
  `rebuild-index` (deliberately late: the index backfills from board history,
  so waiting costs nothing).
- **M7 — Experiments.** `race`/`vote` stages, candidate branches + experiment
  refs (run-ids), tallying + vote telemetry, raced decomposition with apply
  session last. Needs M1 machinery, M6 telemetry, and a trusted basic loop as
  control.
- **Continuous / when-it-bites:** cross-feature drift policy (~M4, when
  parallel features become real), budget caps (needs telemetry), execution
  environments (reserved).

Known behavior-risk hotspots (budget iteration time): M1 `run_session`
semantics, M3 decompose prompt quality.

## Open Questions
- Vote weighting (deferred until per-judge verdict data exists).
- Voice (M5) deferred until the M1–M7 migration completes — see
  docs/plans/026 for what was tried and why.

## Change history
- [2026-06-11] Initial capture of direction discussion (session pipelines, stage
  types, handoff channels, gates/checks manifest, race isolation, MCP strategy,
  board extensions, named principles, gaps register).
- [2026-06-11] Decomposition trigger resolved (user decisions: condensing happens
  client-side with human approval of the doc; decomposition output always lands as
  draft with human promotion). Gap removed; `_meta`-vs-project-level-tasks added to
  open questions.
- [2026-06-11] Conversation frontend direction resolved (user constraints: no
  public exposure, own VPN, conversational voice, read-only repo access for the
  conversing agent). Claude-app connector path ruled out; target is a CLI session
  on the North host with a design-conversation permission profile and a local
  voice MCP (whisper/kokoro). Open: phone-side audio bridge.
- [2026-06-11] Voice setup made concrete (user: North host is headless; all
  devices reach it over LAN/VPN): VoiceMode MCP in the `north-design` session +
  whisper.cpp/Kokoro/LiveKit as systemd user units + browser web frontend with
  TLS as the single audio path for all devices. Phone-audio-bridge open question
  resolved into the LiveKit leg (experimental until proven).
- [2026-06-11] Decomposition flow reworked after recategorization (user):
  conversation outputs are work intake ("task bundles"), not design docs →
  conversations become first-class Borealis objects with frontmatter state;
  pending conversations *are* the decomposition queue (checked before the
  project-task queue); decompose-as-task and the permanent-main-board idea
  dropped; decomposer distills durable decisions into repo docs under a
  docs-only guard; racing decomposition deferred (apply-session shape recorded).
  `_meta`/project-level-tasks open question resolved away.
- [2026-06-11] Cockpit defined as first-class vocabulary (single conversational
  session, understand-and-curate only). Review escalation resolved: Borealis
  event emitter + notifier interface, Telegram first transport (user decision;
  ntfy recorded as the swap candidate after evaluation — comparison:
  privacy/VPN-dependence vs zero-hosting/cloud-transit); service-health
  notifications added (WARNING+
  with dedup + systemd OnFailure); deterministic review brief; conversational
  review in cockpit; verdict verbs CLI-only (user decision: accidental-trigger
  risk); feature threads; refine rule (task added to review feature reverts it
  to in_progress); PR integration declined. Weekend acceptance narrative
  recorded.
- [2026-06-12] Notification transport switched to Telegram-first (user decision;
  ntfy remains the swap candidate post-evaluation). Small questions: handoff-note
  convention adopted; transcript three-tier storage with explicit session
  manifest links adopted. Losing-candidate and squash questions updated with
  detailed proposals pending confirmation.
- [2026-06-12] Losing-candidate policy confirmed with run-id discriminator in
  experiment refs (user caught the re-run collision case) + idempotent race
  setup. No-squash confirmed (first-parent idiom). Chroma setup made concrete
  (single server unit, collection per project, local embeddings, memory MCP
  with one read-only search tool, rebuildable from board history). Open
  questions now down to vote weighting only.
- [2026-06-12] No-agent-self-maintained-memory-plugins rule added (opencode core
  has no built-in memory; plugins exist but conflict with harness-written
  memory design). Build Order roadmap added (M1–M7); per-milestone plans to be
  written just-in-time, 022 = M1 session executor. Direction document
  considered complete pending implementation learnings.
- [2026-06-12] **M1 complete (plan 022, milestone verdict: passed).**
  Implementation learnings fed back into this direction:
  - The "diff non-empty" done-gate earned its place immediately: a local
    model produced zero work yet its task initially passed both gates
    (unchanged repos pass their own checks). The gate is now harness code
    (stage runner fails a walk with no commits).
  - Capability data point for the core trade: `mistral:7b-16k` cannot hold
    the orchestrator seat at all (echoes tool docs, makes no tool calls);
    `opencode-go/minimax-m3` handled implement + review seats well
    (~$0.001/session). "Capable orchestrator, local subagents" stands;
    local models should not appear in M1-style single-seat profiles.
  - opencode 1.15.13 API verified live: blocking message POST, per-message
    token/cost fields, session permission rules, GET-messages transcript
    export, abort semantics — `run_session` semantics risk (named in Build
    Order) is retired.
  - M1 simplifications on record: resume restarts at stage 0 (checkpointing
    at stage boundaries remains designed-but-unbuilt); per-stage failure
    policy hardcoded (retry-once / fail) rather than config.
- [2026-06-12] **M2 complete (plan 023, milestone verdict: passed).**
  Implementation learnings fed back into this direction:
  - Board extensions landed as designed (conversations + pending queue,
    threads, `blocked_reason` with a third value `infra` for Aurora-side
    auth/config blocks, server-enforced draft + promote verbs + status
    transition tables, split with `superseded`). One interaction the
    direction hadn't named: feature→review "all tasks done" must treat
    `superseded` as non-blocking.
  - MCP surface: official `mcp` SDK, one FastMCP server per grant set
    mounted at `/mcp/{grant}/` (trailing slash matters — bare path 307s),
    stateless + JSON responses. Per-grant servers made tool-list filtering
    structural rather than request-time logic. MCP tools call the REST
    API layer under the same board lock — REST-canonical held with zero
    duplication of write logic.
  - Draft gate strengthened beyond the letter of the design: promote is
    the *only* exit from draft (status PATCH/PUT reject it), and a task
    cannot be promoted while its feature is draft.
  - 022 loose end §1 closed: rate-limited tasks requeue as `ready` and
    entering `ready` always clears `ready_at`, so the existing cooldown
    is the backoff.
- [2026-06-12] **M3 complete (plan 024, milestone verdict: passed).** The
  acceptance narrative ran live: conversation shipped → decomposed into
  grounded drafts → human promotion → execution → feature in review
  (~$0.006 total). Learnings fed back:
  - Per-profile MCP grants landed cleaner than designed: opencode's
    message POST accepts a `tools: {pattern: bool}` map, so the decompose
    profile enables `borealis_*` while a global config disable hides board
    tools from every other seat — structural, no deny-pattern fragility.
    The M2 MCP surface spoke opencode's client dialect with zero changes.
  - Decompose seat capability: `minimax-m3` grounds and writes excellent
    task bodies but is slow per step — decompose needs its own timeout
    (1800s) and a "work briskly, stay in your worktree" prompt; with
    those it finishes in ~70s.
  - Operational lesson worth keeping: every long-running runner needs
    (a) backoff computed *after* the run, not from loop-entry time, and
    (b) startup recovery for its in-flight state (stale `decomposing`
    conversations now reset to pending, mirroring stale task recovery).
  - 022 loose end §3 closed: managed clones and feature branches are
    created on demand by code (TaskRunner / ConversationRunner).
  - Durable-decision distillation is prompt-soft: the decomposer may
    task-out the doc write instead of doing it (the docs-only guard +
    result file make the choice visible either way). Acceptable for now.
- [2026-06-12] Post-M3 closeout pass: implemented-reality notes added to the
  Board Extensions, MCP Strategy, and Conversation Intake sections; Build
  Order M1–M3 marked done/passed; loose-end registers added to plans 023/024
  and 022's triaged (§1, §3 closed; §6 half). Plan 025 (M4 human loop)
  written.
- [2026-06-12] **M4 complete (plan 025, milestone verdict: passed).** The
  weekend narrative (text-only) ran live: ship via cockpit grant →
  decompose → drafts notification → promote → execute → question-block →
  notification → answer from cockpit (auto re-ready) → review
  notification carrying the brief line → brief read via cockpit →
  CLI approve → merged + archived (~$0.0045 total). Learnings fed back:
  - The WARNING+ forwarding layer paid for itself on day one: the first
    live approve exposed a latent M2-era bug (feature archival staged
    `archived/` adds but not `active/` deletions → permanently dirty
    board repo → remote sync failing every poll), and it was the
    notification path that surfaced it. The dedupe design got an organic
    live proof at the same time: 13+ identical sync errors → exactly one
    notification.
  - `feature_review` emission moved to "when the brief note lands"
    (single rich notification; the note doubles as the briefed-marker) —
    reconciles "emit where the status change lands" with "notification
    carries the brief summary"; accepted cost: manual review transitions
    notify on Aurora's next tick.
  - Verdict verbs stayed CLI-only with zero friction: the cockpit grant
    (M2's curated set, untouched) was sufficient for every cockpit motion
    in the narrative — ship, promote, answer, read brief.
  - Telegram transport is built but has never fired (user credentials
    pending); transports proved swappable in practice — the whole smoke
    ran on the log transport with identical semantics.
