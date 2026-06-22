# 036 — Borealis vs Backlog.md: In-Depth Comparison & Replacement Audit

> **Type:** Analysis / audit (not an implementation plan). Produced to answer:
> *"Is Backlog.md a viable drop-in replacement for Borealis, and how well does it
> match the Borealis format?"*
>
> **Sources:** Borealis source under `borealis/borealis/service/` (models, board
> parser/loader/writer, api, mcp, orchestrator). Backlog.md from a local clone at
> `tmp/backlog.md` @ `v1.47.0` (commit `fc4d977`) — `src/types/index.ts`,
> `src/markdown/`, `src/mcp/`, `src/core/`, `backlog/config.yml`, real task/decision
> files, `AGENTS.md`/`CLAUDE.md`.

---

## 1. Summary / verdict

Backlog.md and Borealis share a striking amount of **surface DNA**: both are
git-backed, Markdown-with-YAML-frontmatter, one-file-per-item task systems with a
Kanban status model, dependency links, an MCP surface, and a first-class
"AI agents are the primary operators" design philosophy. On the *storage format*
axis they are close cousins.

But they are **not the same kind of system**:

- **Backlog.md is a *record-and-visualise* tool.** It stores tasks and paints
  boards; humans/agents drive every state transition through its CLI/MCP. It has
  no runtime, no scheduler, no executor, no concept of remote project repos.
- **Borealis is a *board service + orchestration spine*.** It is a long-running
  FastAPI process that watches external project git repos, auto-transitions
  feature/task status from git events, computes an eligible run-queue with
  cooldowns and a dependency DAG, gates drafts, and feeds Aurora's AI pipelines
  via role-scoped MCP grants. The board format is only ~30% of what Borealis is.

**Verdict:** Backlog.md is a strong match for Borealis's *board/data layer* and
could replace the **parser/loader/writer/models** with modest effort. It is **not**
a drop-in for Borealis as a whole, because it does not implement the orchestration,
multi-project, git-watcher, decomposition, or REST-service responsibilities that
are the actual reason Borealis exists. Adopting it wholesale would mean deleting
the orchestration layer or re-attaching it on top of Backlog's CLI — at which point
you've kept most of Borealis anyway.

Recommendation in §13.

---

## 2. Identity & tech stack

| | **Borealis** | **Backlog.md** |
|---|---|---|
| Purpose | Board service + AI-pipeline orchestrator (half of "North") | Markdown-native task manager + Kanban visualiser |
| Language / runtime | Python 3.12, FastAPI, `python-frontmatter`, `GitPython` | TypeScript 5 on Bun, single global npm binary |
| Process model | Long-running daemon (systemd service) | Short-lived CLI invocations + optional `browser`/`mcp` servers |
| License | (in-repo, North) | MIT |
| Maturity | Internal, pre-1.0, actively churning | v1.47.0, public, very active, broad install base |
| Distribution | Service in monorepo | `npm i -g backlog.md`, brew, nix |

The runtime mismatch matters: replacing Borealis with Backlog.md means inverting
from a **service you call over HTTP/MCP** to a **CLI you shell out to** (or its MCP
server). Aurora and the `north` CLI currently talk to Borealis's REST API; Backlog
has **no REST API** (see §9).

---

## 3. On-disk storage format

### 3.1 Borealis layout

A **dedicated board git repo** (separate from code repos), structured by project:

```
board/                              # its own git repo (board/.git)
  projects.yaml                     # registry: ssh_url, base_branch, auto_merge
  projects/<project>/
    board/features/active/<feature-id>/
      _feature.md                   # feature frontmatter + description
      tasks/<task-id>.md            # task frontmatter + body
      tasks/<task-id>.result.md     # pipeline result companion
      tasks/<task-id>.thread.md     # comment thread companion
    board/features/archived/<feature-id>/...
    conversations/<id>.md           # inbound conversation
    conversations/<id>.thread.md
    conversations/<id>.result.md
```

### 3.2 Backlog.md layout

A `backlog/` folder **inside the code repo itself**:

```
backlog/
  config.yml                        # statuses, prefixes, git flags, DoD…
  tasks/back-10 - Title.md          # active tasks (human-readable filename)
  drafts/                           # draft tasks awaiting promotion
  completed/back-89 - Title.md      # done tasks (moved here)
  archive/{tasks,drafts}/           # archived
  decisions/decision-1 - Title.md   # ADR-style decisions
  docs/                             # free-form documents
  milestones/                       # milestone definitions
```

### 3.3 Field-level mapping (task)

Borealis `TaskModel` ⇄ Backlog `Task`:

| Borealis field | Backlog equivalent | Notes |
|---|---|---|
| `id` (zero-padded `001`) | `id` (`back-10`, prefix configurable, `zeroPaddedIds`) | Both string IDs; Backlog embeds a configurable prefix, Borealis pads to 3 digits |
| `title` | `title` | ✅ direct |
| `status` (fixed enum) | `status` (configurable string) | **Semantic gap** — see §4 |
| `pipeline` | — | **No equivalent.** Borealis-specific (which AI pipeline runs the task) |
| `depends_on[]` | `dependencies[]` | ✅ direct concept |
| `created_at` | `createdDate` | ✅ (Backlog `date_format` configurable) |
| `ready_at` | — | Borealis cooldown bookkeeping; no equivalent |
| `blocked_reason` (question/dependency/infra) | — | No structured block reason; Backlog uses status only |
| `split_from` | (subtask `parentTaskId`) | Partial — Backlog has hierarchy, not "split" lineage |
| `decomposed_from` | — | Borealis conversation→task lineage; no equivalent |
| `body` (free markdown) | `description` + structured sections | Backlog **parses the body into typed sections** (see §3.4) |
| — | `assignee[]`, `reporter` | Backlog has assignment; Borealis has none |
| — | `labels[]`, `priority`, `milestone`, `ordinal` | Backlog richer on human-PM metadata |
| — | `references[]`, `documentation[]`, `modifiedFiles[]` | Backlog links tasks↔docs↔code |

### 3.4 Body structure

- **Borealis:** `body` is opaque free Markdown; meaning lives in frontmatter.
  Results/threads are *separate companion files* (`.result.md`, `.thread.md`).
- **Backlog:** the body is a **structured, parsed document** with canonical
  sections — `## Description`, `## Acceptance Criteria` (checkbox items parsed into
  `acceptanceCriteriaItems`), `## Implementation Plan`, `## Implementation Notes`,
  `## Final Summary`, and inline `## Comments` (delimited by `---`). Definition of
  Done is a parsed checklist. This is a meaningfully **richer task document model**
  than Borealis, and it is the heart of Backlog's "spec-driven AI" workflow.

**Assessment:** formats are isomorphic enough that a converter is straightforward.
Backlog is the richer schema for the *task document*; Borealis is the richer schema
for *orchestration metadata* (pipeline, ready_at, blocked_reason, decomposed_from).

---

## 4. Status / workflow model

| | Borealis | Backlog.md |
|---|---|---|
| Task statuses | **Fixed enum:** draft, ready, queued, in_progress, done, failed, blocked, superseded | **Configurable list**, default `To Do / In Progress / Done` (`config.yml: statuses`, `default_status`) |
| Feature statuses | **Fixed enum:** draft, open, in_progress, review, merged, closed, blocked | No feature concept (see §5) |
| State machine | **Enforced** by API + orchestrator (promote_draft, draft gating, auto-transitions) | **None** — status is a free string column; any value the user sets |
| Who transitions | Mostly the **system** (resolver/git_watcher) + agents via MCP | Always a **human/agent** via `task edit -s` |

This is the deepest conceptual divergence. Borealis statuses are a **workflow with
rules** (a `queued` task becomes `in_progress` only via the resolver; a `blocked`
task with reason `question` re-readies when answered; merge events drive
`in_progress → review → merged`). Backlog statuses are **board columns** — flexible
and configurable, but with no enforced transitions or automation behind them.

To match Borealis semantics on Backlog you would configure
`statuses: [draft, ready, queued, in_progress, done, failed, blocked, superseded]`
and rebuild the transition rules **yourself** outside Backlog, since Backlog won't
enforce them.

---

## 5. Hierarchy & relationships

| Concept | Borealis | Backlog.md |
|---|---|---|
| Top grouping | **Project** (registered in `projects.yaml`, maps to a code repo) | **None** — one backlog per repo |
| Mid grouping | **Feature** = a **git branch** in the code repo; has its own status, deps, lifecycle | **Milestone** (a label-like bucket) — *not* a branch, no lifecycle |
| Leaf | **Task** under a feature | **Task** (flat, repo-wide) |
| Sub-hierarchy | `split_from` lineage (flat) | **Subtasks** via dotted IDs (`back-1.1`) + `parentTaskId`, arbitrary depth |
| Inbound work | **Conversation** → decomposed into features/tasks | **Draft** → promoted to task |

Borealis's hierarchy is **Project → Feature(=branch) → Task**, purpose-built so a
feature corresponds to a working branch + PR in an external repo. Backlog's model is
**flat tasks (+ optional subtasks) within a single repo**, grouped loosely by
milestone. There is **no project layer and no branch=feature equivalence** — the
single most important structural mismatch for North, where one Borealis instance
fans out across many code repos.

Backlog *does* aggregate task state **across branches** of its host repo
(`cross-branch-tasks.ts`), but that is "find the newest version of this task file
across my branches," not "each feature owns a branch in a remote repo."

---

## 6. Dependencies & scheduling

Both support task dependency edges. The difference is what is *done* with them:

- **Borealis** turns the DAG into an **executable schedule**. `resolver.py`:
  - stamps `ready_at` and promotes `ready → queued` after a configurable cooldown;
  - computes `EligibleTask`s (queued tasks whose deps are satisfied), ordered by
    DAG depth — i.e. an actual **run-queue** the orchestrator consumes (`get_queue`).
- **Backlog** computes **sequences** (`src/core/sequences.ts`): groups of tasks
  that *could* run in parallel given dependencies. This is **advisory** — a
  visualisation/planning aid. Nothing executes them; no cooldown, no queue, no
  notion of "currently running."

So dependencies map cleanly as *data*, but Borealis's scheduling **behaviour** has
no counterpart in Backlog.

---

## 7. Interfaces

| Interface | Borealis | Backlog.md |
|---|---|---|
| REST API | ✅ **Canonical** (FastAPI: tasks/features/comments/conversations) | ❌ **None** |
| MCP | ✅ Per-grant servers mounted at `/mcp/{grant}`, bearer tokens, 4 role grant-sets | ✅ `backlog mcp` (stdio + HTTP w/ bearer/basic auth, DNS-rebinding protection) |
| CLI | ❌ (the separate `north` CLI calls the REST API) | ✅ **Primary** surface (`task create/edit/list`, `board`, `search`, …) |
| TUI | ❌ | ✅ `backlog board` (live Kanban in terminal) |
| Web UI | partial (`cockpit/`) | ✅ `backlog browser` (drag-drop Kanban, real-time) |

**Critical:** the integration contract is inverted. North today is built around
Borealis's **REST API as the canonical surface** with MCP as a secondary view
("REST stays canonical; MCP is a surface, not the spine" — `mcp.py`). Backlog has
**no REST API at all** — its canonical surface is the **CLI**, with MCP as the
programmatic path. Any consumer of Borealis's HTTP endpoints (Aurora, the `north`
CLI, cockpit) would have to be rewritten to shell out to `backlog` or speak its MCP.

### MCP tool surfaces compared

- **Backlog tools:** `task_create/edit/list/view/search/complete/archive`,
  `document_*`, `decision`(via docs), `milestone_*`, `definition_of_done_*`,
  `get_backlog_instructions`. Rich on **task documents, docs, decisions, milestones**.
- **Borealis tools:** `get_queue`, `list/get` features/tasks/conversations,
  `get_review`, `pending_conversations`, `create_feature/task`, `split_task`,
  `promote_draft`, `add_comment`, `create_conversation`. Rich on **orchestration,
  queue, decomposition, review, role grants**.

They barely overlap beyond CRUD — reflecting the record-vs-orchestrate split.

---

## 8. AI-agent integration

Both are explicitly agent-first; the *shape* differs.

- **Backlog.md:** ships instruction files (`AGENTS.md`/`CLAUDE.md` symlinked) and
  MCP **workflow resources** (`backlog instructions overview / task-creation /
  task-execution / task-finalization`). The workflow is **spec-driven**: agent
  reads overview → creates well-formed task (description + acceptance criteria) →
  plans → implements → records implementation notes + final summary → completes.
  Agents self-assign (`-a @claude`). Strong "one agent, structured single task"
  loop. **No multi-role / permission model** beyond MCP auth.
- **Borealis:** integration is **role-scoped via MCP grants** —
  `decomposer / implementer / reviewer / cockpit`, each granted a different tool
  subset (`mcp.py: GRANTS`). This encodes a **multi-agent pipeline**: a decomposer
  turns conversations into features/tasks, an implementer runs tasks and splits
  oversized ones, a reviewer promotes drafts and answers questions. This is tied to
  Aurora's pipeline runtime.

**Assessment:** Backlog has the **better single-task agent ergonomics** (acceptance
criteria, DoD, plan/notes are excellent for predictable agent output). Borealis has
the **multi-agent orchestration model** Backlog lacks. The ideal is arguably
Backlog's task-document discipline *inside* Borealis's role/pipeline structure.

---

## 9. Git strategy

| | Borealis | Backlog.md |
|---|---|---|
| Where the board lives | **Separate dedicated git repo** | **Inside the code repo** (`backlog/`) |
| External repos | Tracks **many** project repos via `projects.yaml` (ssh_url, base_branch, auto_merge) | **None** — single host repo only |
| Commits | Every mutation auto-commits with structured messages (`[system:task] queued 003`), pushes to origin with rebase-on-conflict | `auto_commit` (default off); `bypass_git_hooks`; `filesystem_only` mode |
| Git as signal | **Yes** — `git_watcher.py` polls project repos; HEAD movement / merges drive feature/task status transitions | **No** — git is storage + cross-branch state resolution only |
| Cross-branch | N/A (board is single-branch) | Resolves a task's latest state across active branches (`check_active_branches`, `active_branch_days`) |

This is a fundamental architecture divergence. Borealis is a **hub** that observes
N external repos and reacts to their git history. Backlog is **self-contained** in
one repo and never reaches outside it. North's whole premise (one board fanning out
to multiple project repos, reacting to merges) has **no analogue** in Backlog.

---

## 10. Orchestration & automation (Borealis-only)

The following Borealis capabilities have **no equivalent** in Backlog.md. They are
the substance of Borealis beyond storage:

- **Run-queue resolver** — cooldown, DAG-depth ordering, eligibility (`resolver.py`).
- **Git watcher / supervisor** — polls project repos, reloads board on HEAD change,
  auto-transitions status from git events (`orchestrator/`).
- **Pipelines** — each task names a `pipeline`; Aurora executes it. No Backlog field.
- **Conversations → decomposition loop** — inbound conversation (pending →
  decomposing → decomposed) becomes features/tasks; `decomposed_from`/`decomposed_into`
  lineage. Backlog's nearest concept (drafts) is far simpler.
- **Comment threads with behaviour** — `[question]/[answer]/[note]` entries in
  `.thread.md`; answering a question-blocked task **re-readies** it. Backlog has
  flat task comments with no status side-effects.
- **Draft gating** — features/tasks land `draft` and must be promoted before they
  can run; enforced by the service.
- **Result companions** — `.result.md` capturing pipeline output per task.
- **Role-grant MCP** — defense-in-depth tool scoping per agent role.
- **Multi-project registry & auto-merge** — `projects.yaml`.

Backlog's automation surface, by contrast, is essentially: `onStatusChange`
callback command, `definition_of_done` checklist defaults, and auto-commit. It
is deliberately a **thin, local tool**, per its own `CLAUDE.md` ("avoid extra
layers… unless there is an immediate, proven need").

---

## 11. What Backlog.md does *better* than Borealis

To be even-handed — these are real wins Borealis could learn from:

1. **Structured task document** — acceptance criteria, DoD, implementation
   plan/notes, final summary as first-class parsed sections. Excellent for agents.
2. **Human-readable filenames** (`back-89 - Add-dependency-parameter.md`) aid
   discovery vs Borealis's `003.md`.
3. **Configurable statuses & ID prefixes** — no code change to re-label columns.
4. **Built-in TUI + web Kanban + fuzzy search** across tasks/docs/decisions.
5. **Decisions (ADRs) and Documents** as first-class, searchable entities.
6. **Assignees, priority, labels, milestones** — richer human-PM metadata.
7. **Maturity** — v1.47, MIT, large test suite, active maintenance, broad install.
8. **Subtasks** with arbitrary depth and parent/child summaries.

---

## 12. Feature-parity matrix

Legend: ✅ full · 🟡 partial/different · ❌ absent

| Capability | Borealis | Backlog.md |
|---|---|---|
| Git-backed Markdown+frontmatter store | ✅ | ✅ |
| One file per task | ✅ | ✅ |
| Kanban status model | ✅ (fixed) | ✅ (configurable) |
| Enforced status state-machine | ✅ | ❌ |
| Task dependencies | ✅ | ✅ |
| Dependency-driven run-queue | ✅ | 🟡 (advisory sequences only) |
| Feature = git branch | ✅ | ❌ |
| Multi-project (many repos) | ✅ | ❌ |
| Watches external repos / reacts to git | ✅ | ❌ |
| Conversations / decomposition | ✅ | 🟡 (drafts only) |
| Comment threads w/ status effects | ✅ | 🟡 (flat comments) |
| Pipelines / agent execution binding | ✅ | ❌ |
| Role-scoped MCP grants | ✅ | 🟡 (auth only, no roles) |
| REST API | ✅ | ❌ |
| MCP server | ✅ | ✅ |
| CLI | 🟡 (separate `north`) | ✅ |
| TUI / Web Kanban | 🟡 (cockpit) | ✅ |
| Fuzzy search | ❌ | ✅ |
| Structured AC / DoD / impl-notes | ❌ | ✅ |
| Decisions (ADR) / Documents | ❌ | ✅ |
| Assignee / priority / labels / milestones | ❌ | ✅ |
| Subtasks | 🟡 (split lineage) | ✅ |

---

## 13. Replacement options & assessment

### Option A — Full drop-in replacement
Replace Borealis with Backlog.md and point Aurora/`north` at it.
- **Blockers:** no REST API; no multi-project; no feature=branch; no git-watcher;
  no resolver/queue; no pipelines; no conversation/decomposition; no role grants.
- **Effort:** very high — you'd rebuild ~70% of Borealis (orchestration) *on top
  of* Backlog's CLI, plus rewrite every REST consumer to shell out / use MCP.
- **Verdict:** ❌ Not viable as "drop-in." You'd lose North's defining behaviour.

### Option B — Adopt Backlog's *format & task-document model*, keep Borealis service
Re-base Borealis's parser/loader/writer/models on Backlog's conventions
(structured AC/DoD/plan/notes sections, human-readable filenames, configurable
statuses, decisions/docs), and optionally embed Backlog as a read/visualise layer.
- **Effort:** moderate — rewrite the `board/` module + models; keep API, MCP,
  orchestrator, git_watcher, projects.yaml.
- **Gain:** richer task docs, TUI/web boards "for free," ADRs/docs, search.
- **Risk:** Backlog's single-repo, no-feature, no-project assumptions fight North's
  multi-repo model; you'd be using its file format but not its tool.
- **Verdict:** 🟡 The most pragmatic "borrow the good parts" path.

### Option C — Use Backlog.md as the *task-document layer within a feature*, Borealis as the hub
Keep Borealis as the multi-project orchestration spine; let each feature's tasks be
authored/maintained in Backlog's format so agents get its task-execution workflow
and AC/DoD discipline.
- **Effort:** moderate; mostly a format + agent-instruction change.
- **Verdict:** 🟡 Viable if the goal is "better agent task ergonomics," not
  "replace Borealis."

### Option D — Status quo
Keep Borealis; cherry-pick specific ideas (structured AC/DoD sections,
human-readable filenames, configurable statuses, a TUI) as incremental Borealis
features.
- **Verdict:** ✅ Lowest risk; captures most of the upside without an architecture
  rewrite. Aligns with the in-progress status/dependency redesign already on the
  roadmap.

---

## 14. Recommendation

**Do not treat Backlog.md as a drop-in replacement** — it solves a narrower problem
(single-repo task recording + visualisation) than Borealis (multi-repo board service
+ AI orchestration spine). The "10 alternatives" finding stands: nothing replaces
Borealis wholesale because the git-watcher + multi-project + pipeline orchestration
is the unusual, valuable part, and Backlog has none of it.

**Best value:** pursue **Option D with selective Option B borrowing.** Concretely,
fold these Backlog.md ideas into the planned Borealis status/dependency redesign:

1. Structured task body sections (Acceptance Criteria + Definition of Done +
   Implementation Plan/Notes/Final Summary) — biggest agent-quality win.
2. Human-readable task/feature filenames.
3. Configurable status list instead of a hard-coded enum (keeps your state-machine,
   removes the code-change-to-rename friction).
4. First-class Decisions (ADR) and Documents entities.
5. A read-only TUI/web Kanban (or evaluate embedding `backlog browser` against an
   exported view) instead of building cockpit from scratch.

Keep Borealis's REST-canonical service, multi-project registry, git_watcher,
resolver/queue, decomposition loop, and role-grant MCP — these have no substitute.

---

## 15. Risks & caveats

- **API-contract inversion** (REST→CLI) is the single biggest integration cost if
  any Backlog adoption goes deep; budget for it explicitly.
- **Backlog is intentionally minimal** — its own guidelines resist added layers, so
  upstreaming Borealis's orchestration into it is a non-starter; the dependency
  would be one-directional (we adopt its format, not extend its tool).
- **Single-repo assumption** is baked deep in Backlog (cross-branch resolution,
  `backlog/` in-repo). Bending it to North's multi-repo hub model is friction-heavy.
- **Format drift** — if we adopt Backlog's file conventions but not its CLI, we own
  keeping the parser compatible with their evolving schema (no stability guarantee:
  their `CLAUDE.md` explicitly disclaims source-level API stability).
- This audit reflects Backlog.md **v1.47.0**; re-verify before any commitment.

---

## Change history

- [2026-06-17] Initial audit written. Borealis analysed from source; Backlog.md
  analysed from local clone `tmp/backlog.md` @ v1.47.0 (`fc4d977`). No code changed.
