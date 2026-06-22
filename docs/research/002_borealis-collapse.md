# Research: Collapsing Borealis into a Library (brainstorm)

> **Note on this file:** Captured during a voice brainstorm on 2026-06-17. Plan
> mode only permits writing to this plan file, so the research lives here for
> now. **On approval, move/rename this to `docs/research/002_borealis-collapse-to-library.md`.**
> This is exploratory ("theory land") — no decisions are final.

## Context

Sam wants to simplify Borealis toward an MVP: get one thing working really well
and refined for this application, rather than building everything before we know
what we need. The current model feels over-built — the CLI takes up to five verbs
to reach a target, and there are two parallel status state machines (feature and
task). Inspiration is being drawn from `backlog.md` (see
`docs/research/001_borealis-vs-backlogmd-analysis.md`).

## Current model (as-is)

Three-level hierarchy: **Project → Feature → Task**
(`borealis/borealis/service/models.py`).

- `ProjectModel` — name, ssh_url, base_branch, auto_merge, `features` dict,
  `conversations` dict. (`models.py:101`)
- `FeatureModel` — feature_id, title, status, feature_path, description,
  depends_on, created_at, merged_at, decomposed_from, `tasks` dict. The
  `feature_id` **is** the git branch name (`.branch` property). (`models.py:62`)
- `TaskModel` — task_id, title, status, pipeline, task_path, depends_on,
  created_at, ready_at, blocked_reason, split_from, decomposed_from, body.
  (`models.py:46`)
- Two status enums: `TaskStatus` (8 states) and `FeatureStatus` (7 states).

Storage: a **separate dedicated board git repo**, Markdown + YAML frontmatter,
laid out under `projects/<project>/board/features/...`. Orchestration runs as a
**service**: a git-watcher that reacts to push/merge events to auto-transition
status, plus a resolver that promotes tasks ready→queued on a timer and computes
the run queue (`orchestrator/resolver.py`).

## The emerging direction

### 1. Collapse the hierarchy
- **Drop the feature record.** A feature becomes just a **label string on tasks**
  that also names the branch. You find a feature by filtering tasks on that label.
- **Feature status is derived, not stored** — it's an aggregate of its tasks'
  statuses plus whether the branch merged. This removes the entire `FeatureStatus`
  state machine.
- **Feature-level `depends_on` collapses into task-level `depends_on`.**
- Accepted trade-off: a feature no longer owns its own title/description body;
  it's purely an emergent grouping. Consistency risk: the feature label is now
  denormalized across tasks and can drift if mislabeled (acknowledged, accepted
  for MVP).

### 2. Drop the network service
- No cross-device use → no reason for a network service or persistent process.
- No git-watcher: **local checkout state *is* the board state.** Whatever is
  present locally is the truth; nothing needs to constantly poll git.
- Borealis becomes a **plain library / package** with a well-defined public
  interface for the core task operations.
- Fork off the library: a **CLI**, a **TUI**, an **MCP server**, and **Aurora
  imports it directly**. Tight coupling to Aurora is acceptable — serving Aurora
  is the whole point of Borealis.

### 3. Projects become a home-dir registry
- A `~/.north` config holds a list of projects mapping **name → absolute path on
  disk**. Add/remove freely.
- The CLI lets you select which project you're working in; you write tasks/
  features and everything else is built implicitly. Goal: `task add` just adds a
  task to the board in the project repo — that's it.

## Open question: where does the board physically live? (UNRESOLVED)

Sam's sketch: draft tasks on `main` (or a dedicated organizing branch), commit
them, queue them, create a feature branch matching the feature, do the tasks on
that branch, then merge back into main/organizing branch.

**Holes poked in the "board lives on the working branch" approach:**
1. **Divergence** — status updates happen on the feature branch while main holds
   the stale version; the real status ends up scattered across whichever branches
   are checked out, contradicting the single-source-of-truth goal.
2. **Merge conflicts** — drafting new tasks on main while a branch updates task
   status produces YAML frontmatter conflicts on the same files at merge time.
3. **Ambiguous identity** — "local state is the board," but local state depends
   on which branch is checked out, so the board changes identity as you switch
   branches.
4. **Aurora consumption** — reading the board from main misses live branch
   progress; reading from a branch sees only one feature's slice.

**Proposed resolution (not yet decided):** decouple the board from the code
repo's branches — store it once in a fixed location that doesn't move when you
switch branches. Options:
- the `~/.north` home dir, keyed per project (local sidecar), or
- a dedicated **orphan branch** in the repo never checked out for work.

The original separate-board-repo design avoided exactly these problems; that
instinct was right even if the network service was overkill. **Sam has not yet
decided** whether keeping the board out of the working branches is acceptable.

## Next steps when we resume
1. Decide board storage location (sidecar in `~/.north` vs. orphan branch vs.
   on-branch-with-rules).
2. Confirm whether *any* always-on orchestration survives, or whether ready→
   queued promotion and queue computation move into Aurora / become on-demand.
3. Define the library's public interface (core task operations).
4. Decide fate of `conversations` and the `pipeline` field under the new model.
5. Then write a proper implementation plan under `docs/plans/`.
