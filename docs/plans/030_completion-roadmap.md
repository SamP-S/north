# 030 — Completion Roadmap (demo proof → metrics → memory → experiments)

## Context

North is complete and live-proven through milestone **M4** of the Build Order in
[021_session-pipeline-architecture.md](021_session-pipeline-architecture.md):
the session-executor spine (M1), board/MCP surface (M2), decomposition loop
(M3), and human loop (M4) all run end to end. The codebase is clean (302 tests
passing, zero `TODO`/`FIXME`/`NotImplementedError`; the only placeholder is the
deliberate no-op post-commit hook in `aurora/aurora/service/git/hooks.py`,
reserved for the planned Reconciliation feature).

What remains is the tail of the roadmap plus the operational features stripped
in v1. This document is the **umbrella roadmap** for that remaining work, in the
user-chosen order. Per the project's just-in-time convention, each phase below
gets its own detailed numbered plan (031–034) written immediately before it is
implemented; this plan is the durable index that says *what*, *why*, *where the
docs live*, and *what new docs to write*.

Source documents this roadmap draws from:

- [021_session-pipeline-architecture.md](021_session-pipeline-architecture.md) §"Build Order (roadmap)" — M5–M7 definitions and milestone verdicts.
- [../design/99_planned-features.md](../design/99_planned-features.md) — Advanced Testing, Usage Caps/Budget/Metrics, Memory, Experiments (the §source-of-truth for scope of phases 2–4).
- [../design/01_v2-architecture.md](../design/01_v2-architecture.md) — current system architecture.
- [../../aurora/tests/smoke/SMOKE_TEST.md](../../aurora/tests/smoke/SMOKE_TEST.md) — the existing manual checklist phase 1 supersedes.
- `README.md` §"Session execution", §"Gates and the checks manifest" — pipeline/gate behaviour the demo exercises.

## Scope (four phases, in order)

1. **Full pipeline usage with a demo calculator project** — a real, throwaway
   target repo that proves the whole loop (conversation → decompose → execute →
   gate → review → merge) on something other than North itself. Closes the
   "never battle-tested" gap. → plan **031**.
2. **Budgeting / metrics** — aggregate and persist the per-session token/cost
   data Aurora *already captures*, add monthly spend tracking and soft-cap
   enforcement. → plan **032**.
3. **M6 Memory (ChromaDB)** — per-project long-term memory so stateless sessions
   stop re-deriving context; completion-time indexer + `search_history` MCP. →
   plan **033**.
4. **M7 Experiments** — `race`/`vote` stages, candidate branches, vote
   telemetry. Depends on phases 2 (telemetry) and 3 (memory). → plan **034**.

Out of scope for this roadmap: M5 Voice (deferred, see
[026_voice.md](026_voice.md)), Frontend UI, LangGraph integration, full
Reconciliation — all documented in `99_planned-features.md`, revisited later.

---

## Phase 1 — Full pipeline usage with demo calculator project

**Goal.** A reproducible end-to-end proof on a trivial but real target repo: a
Python calculator. Exercises every seam the smoke checklist only describes
manually, and gives us a permanent regression target that isn't North itself.

**Why first.** The engine is built but has only ever run its happy path live.
A demo project that we control end to end lets us drive the hard cases
(dependency ordering, gate failure, review verdicts) deterministically before we
build new machinery on top.

**Current reality to build on.**
- Pipelines are linear stage lists: `aurora/definitions/pipelines/default.yaml`
  (cloud) and `local.yaml` (ollama). Stages are `run` (a profile session) and
  `gate` (named checks). See `README.md` §"Pipelines".
- Gates resolve abstract check names against `.north/checks.yaml` in the *target*
  repo (`aurora/aurora/service/stages/gates.py:18`, `CHECKS_MANIFEST`). The demo
  repo must ship this file.
- Projects are registered in the board's `projects.yaml`
  (`borealis/borealis/service/board/parser.py:162`); tasks/features live as
  markdown under the board repo.
- The conversation→drafts→promote→execute→review path is live
  (plans 023–025); phase 1 *uses* it, it builds no new product code.

**What needs doing.**
- Create a standalone **demo calculator target repo** (separate git repo, the
  thing Aurora clones and works in — *not* the board repo). Minimal:
  `calculator.py` with a couple of functions, `pyproject.toml`, a `tests/`
  dir, and crucially a `.north/checks.yaml` mapping `test`/`lint`/`build` to
  real commands. Seed it so there is genuine work to do (e.g. ship `add`/`sub`,
  leave `mul`/`div` as the tasks).
- Decide where it lives: a `demos/calculator/` directory in this monorepo as a
  template, plus instructions to push it to its own repo for registration.
- Author a **scripted end-to-end walkthrough** that replaces the manual smoke
  checklist for this scenario, covering at minimum:
  - register the demo project (`borealis` project register flow),
  - ship a conversation through the cockpit → decompose → promote drafts,
  - two tasks with a `depends_on` ordering, watch the dependency gate hold,
  - one task whose gate **fails** on purpose (broken test) → task `failed`,
  - one feature carried to **review** → brief generated → human verdict → merge,
  - confirm archive + board state after merge.
- Capture which existing tests this scenario overlaps (the
  `aurora/tests/integration/test_full_task_run.py` family) and note gaps where a
  new integration test should be added rather than relying on manual runs.

**Files / locations.**
- New: `demos/calculator/` (target-repo template, incl. `.north/checks.yaml`).
- New doc: `docs/plans/031_demo-calculator.md` (the detailed phase plan + the
  scripted walkthrough, or link to a runbook under `docs/runbooks/`).
- Update: `aurora/tests/smoke/SMOKE_TEST.md` to reference the demo project as the
  canonical target instead of an unspecified "demo project".
- Possibly new: `aurora/tests/integration/test_demo_*.py` for any seam the
  walkthrough shows is untested.

**Docs to write.** Plan 031 + a runbook describing how to stand up the demo from
scratch (env, registration, expected transitions). The runbook is the artifact
the smoke checklist's vague steps become concrete in.

---

## Phase 2 — Budgeting / metrics

**Goal.** Know what every run costs, persist it, and stop runaway spend. North
burns Agent-SDK / subscription credits today with **no spend tracking and no
caps** — flying blind.

**Current reality to build on (important — partial machinery already exists).**
- Per-session usage **is already captured**: `SessionUsage`
  (`aurora/aurora/service/runtime/session.py:28`) holds `input_tokens`,
  `output_tokens`, `reasoning_tokens`, `total_tokens`, `cost`, and
  `_extract_usage` (`session.py:91`) pulls it from the opencode runtime reply.
  `SessionResult` carries it and the README already advertises tokens/cost in
  the per-task session manifest.
- What is **missing**: aggregation across the sessions of a task/feature,
  durable storage, monthly spend accounting, and any enforcement. `TaskState`
  (`aurora/aurora/service/models.py`) has **no** token/cost fields — usage dies
  with the in-memory `SessionResult`.

**What needs doing** (scope source: `99_planned-features.md` §"Usage Caps,
Budget Management, and Metric Collection"):
- **Spend tracking.** A `SpendTracker` persisting monthly spend to JSON under
  `aurora_home` (`load()`, `reset_if_new_cycle(billing_cycle_day)`,
  `add_cost(usd)`, atomic write). Accumulate `SessionUsage.cost` into it after
  each session.
- **Caps + enforcement.** Config fields on `aurora/aurora/service/config.py`:
  `monthly_sdk_credit_usd`, `monthly_soft_cap_usd`, `billing_cycle_day`,
  `max_budget_usd_per_call`; optional per-profile `max_budget_usd` frontmatter.
  Supervisor pauses the runner when `total_usd >= monthly_soft_cap_usd`.
- **Notifications (transport already exists, `aurora/aurora/service/notify.py`).**
  Add `notify_approaching_soft_cap(spent, cap)` and `notify_rate_limit(detail)`.
  Rate-limit detection already requeues; wire it to spend tracking.
- **Surface it.** `/api/status` exposes `monthly_credit_usd`,
  `monthly_soft_cap_usd`, current spend; the `aurora status` CLI displays them.
- **Per-task telemetry.** Add token/cost fields to `TaskState` and thread
  `SessionUsage` up into them so a task's total is queryable.
- **Longer term (optional in this phase, can defer to its own follow-up):** a
  `task_runs` SQLite table (one row per step invocation) with the field list in
  `99_planned-features.md`, plus a daily SQL dump committed to the aurora repo.

**Files / locations.**
- New: `aurora/aurora/service/spend.py` (or `metrics/`), `SpendTracker`.
- Modify: `config.py` (cap fields), `models.py` (`TaskState` token/cost fields),
  `runtime/session.py` / `stages/runner.py` (thread usage up), `notify.py`
  (two new events), the supervisor (soft-cap pause), `service/api/` status
  endpoint, `aurora/aurora/cli/commands/observe.py` (status display).
- New doc: `docs/plans/032_budget-metrics.md`.

**Docs to write.** Plan 032. Update `README.md` with a "Budget & metrics"
section (env vars, where the spend file lives, what the cap does). The detailed
field list and notification call-sites already exist verbatim in
`99_planned-features.md` — plan 032 should reference it, not duplicate it.

---

## Phase 3 — M6 Memory (ChromaDB)

**Goal.** Give stateless sessions durable, per-project memory so the assistant
stops re-deriving the same context every run. This is the milestone that most
makes the system "feel finished," and it unblocks phase 4.

**Why after metrics.** Memory's value is realised by experiments (M7), which
also need telemetry from phase 2; doing metrics first means M7 has both
dependencies ready.

**Scope source.** [021] Build Order §"M6 — Memory" and
`99_planned-features.md` §"Memory". The roadmap specifies:
- A **Chroma** unit (vector store) — decide systemd unit vs. embedded, mirroring
  how opencode/ollama are treated (external optional provider, per plan 029's
  philosophy — North should not hard-own heavyweight deps it can avoid).
- A **completion-time indexer**: when a task/feature completes, index its
  handoff notes, gate reports, and result into the store keyed by project.
- A **`search_history` MCP tool** on the Borealis MCP surface
  (`borealis/borealis/service/mcp.py` — add alongside the existing curated verbs;
  respect per-grant filtering) so sessions and the cockpit can query memory.
- A **`rebuild-index`** command (deliberately late in the roadmap: the index
  backfills from board history, so building it last costs nothing) — likely a
  CLI subcommand that re-scans completed work into the store.

**What needs doing.**
- Choose embedding + store deployment (Chroma embedded vs. served); document the
  decision and whether ollama provides embeddings or a dedicated model does.
- Indexer hook at completion sites (task → `done`, feature → `merged`). The
  notification gate-event sites from plan 025 are the natural seams.
- MCP verb + per-profile grant wiring (the decompose/review/cockpit grants are
  the consumers; default execution seats may or may not get it — decide).
- `rebuild-index` CLI.
- Tests: indexer round-trip, search relevance smoke, grant filtering.

**Files / locations.**
- New: a memory module (store client + indexer), likely
  `borealis/borealis/service/memory/` (board service owns history) or aurora-side
  if indexing happens at execution completion — **open decision for plan 033**.
- Modify: `borealis/borealis/service/mcp.py` (new verb + `_ALL_TOOLS`/grant map),
  completion sites, a CLI command module, `config.py` (store URL/path).
- New: `systemd/` unit if Chroma is served; `scripts/install.sh` wiring (warn,
  don't hard-require — follow plan 029's optional-provider pattern).
- New doc: `docs/plans/033_memory-chroma.md`.

**Docs to write.** Plan 033 + a design note (extend `01_v2-architecture.md` or a
new `docs/design/02_memory.md`) covering the store choice, the index schema
(what fields/metadata per record), and the retrieval contract `search_history`
exposes. Update `README.md` with a Memory section and the new optional dep.

---

## Phase 4 — M7 Experiments

**Goal.** Let the engine run competing attempts and pick a winner: `race`/`vote`
stages, candidate branches, experiment refs, vote tallying + telemetry.

**Why last.** [021] states it explicitly: M7 "needs M1 machinery, M6 telemetry,
and a trusted basic loop as control." It depends on phase 2 (vote/cost
telemetry), phase 3 (memory as context for judges), and on phases 1's demo as
the control to compare experiments against.

**Scope source.** [021] Build Order §"M7 — Experiments" and
`99_planned-features.md` (Parallel/Concurrency caveat: local GPU is single-model
6 GB, so racing local models is throughput-bound — `RUNNER_CONCURRENCY=1` today;
racing may need cloud seats or sequential candidates until throughput is
characterised — flag this as an open constraint).

**What needs doing.**
- New stage types beyond `run`/`gate`: `race` (N candidate sessions) and `vote`
  (judge/tally). This touches the pipeline schema
  (`aurora/aurora/service/stages/pipeline.py`, currently strictly linear
  `run`+`gate`) and the runner.
- **Candidate branches + experiment refs** (run-ids) in git integration so each
  candidate's work is isolated and addressable.
- **Vote telemetry** (depends on phase 2's metric store): per-judge verdicts,
  tally records. [021] Open Question: vote weighting deferred until per-judge
  verdict data exists — so ship unweighted first.
- **Raced decomposition** with an apply session last (from [021] §M7).
- Tests + a demo experiment run against the phase-1 calculator as control.

**Files / locations.**
- Modify: `aurora/aurora/service/stages/pipeline.py` (stage schema),
  `stages/runner.py` (race/vote execution), `git/worktree.py` & `git/features.py`
  (candidate branches/refs), the metric store from phase 2 (vote telemetry).
- New: pipeline definition(s) under `aurora/definitions/pipelines/` exercising
  race/vote.
- New doc: `docs/plans/034_experiments.md`.

**Docs to write.** Plan 034 + a design note on the experiment model (how a
candidate branch/ref is named, how a vote is recorded, the judge contract).
Update `README.md` §"Pipelines" once `race`/`vote` are real stage types, and
resolve the "Vote weighting" Open Question in [021] when data exists.

---

## Files to modify (roadmap-level summary)

| Area | Phase | Key paths |
|------|-------|-----------|
| Demo target repo + runbook | 1 | `demos/calculator/`, `aurora/tests/smoke/SMOKE_TEST.md`, `aurora/tests/integration/` |
| Spend/metrics | 2 | `aurora/.../runtime/session.py`, `models.py`, `config.py`, `notify.py`, supervisor, API status, `cli/commands/observe.py`, new `spend.py` |
| Memory | 3 | new memory module, `borealis/.../mcp.py`, completion sites, `config.py`, `systemd/`, `scripts/install.sh` |
| Experiments | 4 | `aurora/.../stages/pipeline.py`, `stages/runner.py`, `git/`, new pipeline defs |
| Docs | all | `docs/plans/031–034`, `docs/design/`, `README.md` |

## TODO (roadmap progress)

Each item below is "write the detailed plan, then execute it." Detailed plans
are written just-in-time per CLAUDE.md.

- [ ] 1. Phase 1 — write `031_demo-calculator.md`; build `demos/calculator/`; run the scripted end-to-end walkthrough; close gaps with integration tests; update `SMOKE_TEST.md`.
- [ ] 2. Phase 2 — write `032_budget-metrics.md`; implement `SpendTracker` + caps + soft-cap pause; thread `SessionUsage` into `TaskState`; surface in `/api/status` + CLI; add notifications; (optional) `task_runs` SQLite.
- [ ] 3. Phase 3 — write `033_memory-chroma.md` + memory design note; decide store deployment; build indexer + `search_history` MCP + `rebuild-index`; wire grants; tests.
- [ ] 4. Phase 4 — write `034_experiments.md` + experiment design note; add `race`/`vote` stages; candidate branches/refs; vote telemetry; demo experiment against the phase-1 control.

## Change history

- [2026-06-15] Initial roadmap captured. Order set by user: demo proof →
  budget/metrics → M6 memory → M7 experiments. Grounded against current code:
  per-session usage already captured in `SessionUsage` (phase 2 is aggregate +
  persist + cap, not from scratch); pipelines strictly linear `run`+`gate`
  today (phase 4 extends the schema); Telegram transport already live (phase 2
  adds two events, not the transport). Detailed plans 031–034 to follow
  just-in-time.
