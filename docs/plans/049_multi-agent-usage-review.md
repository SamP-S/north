# 049 — Multi-agent usage: design review

**Status:** review / decision document (not yet an accepted plan)
**Date:** 2026-07-10
**Scope:** How North should support 5–10 AI agents (e.g. Claude Code in
separate tmux panes) sharing one board, with the smallest possible new
surface. Decides the coordination primitive and the topology guidance;
does **not** yet commit to an implementation plan (that becomes 050 if we
accept a direction here).

---

## 1. The question

The user opens several tmux panes, each running an agent with the North
skill installed. Every agent wants to: find a task that is workable now,
start it without another agent also starting it, do the work, and record
the outcome — 5–10 agents against **one board**.

The user's leaning (which this review largely endorses, with one addition):
**no separate "claim" data model.** Instead, split work-picking into two
commands:

- one that **shows** the next available task (read-only peek), and
- one that **shows and takes** it (atomic claim).

This document tests that leaning against the concurrency reality, surveys
the alternatives and prior art, works through every deployment topology,
and lands on a concrete recommendation plus the open questions we still
have to answer.

---

## 2. What actually breaks today (the concurrency gap)

North already has real concurrency machinery: an advisory file lock
(`north/.lock`, `O_CREATE|O_EXCL`, stale-steal after 10s — `internal/board/lock.go`)
that serialises each *individual* mutation, and a tolerant snapshot load so
one bad file never breaks the board. Duplicate ids from merges are healed by
`north doctor --fix`. That is a solid base. But it does **not** close the gap
that matters for multi-agent work.

The skill's documented work loop is:

```bash
north task list --status ready --deps met --plain   # 1. what's workable?
north task view 12 --json                            # 2. read the brief
north task move 12 in_progress                       # 3. "claim" it
```

The bug is between steps 1 and 3. The `.lock` makes step 3 atomic *on its
own*, but it does **not** span the read-decide-write across steps 1→3. This
is a classic **TOCTOU** (time-of-check to time-of-use) race:

- Agent A lists ready tasks, sees `12` at the top.
- Agent B lists ready tasks, also sees `12` at the top.
- Both run `move 12 in_progress`. The first wins; the second is a **no-op**
  (`SetStatus` returns early when `target == task.Status`,
  `tasks.go:458`). Neither call errors.
- **Both agents now believe they own task 12** and do the work twice.

`move` doesn't even set `assignee`, so there is nothing recording *who*
took it. Even if each agent followed with `edit 12 --assignee A`, that is a
*second* unlocked write — last-writer-wins on the file, so the frontmatter
says B while A is also working it. At 2–4 agents you might get lucky; at
5–10 picking from the same short ready-queue, collisions are frequent.

**Conclusion:** the missing primitive is a *single operation that selects
and claims under one lock hold*. Everything else (identity, crash recovery,
worktrees) is secondary to closing this window. This is exactly the
"shows-and-takes" command the user proposed — and the reason it must be one
command, not two calls the agent stitches together.

---

## 3. Prior art (and why we won't copy it wholesale)

| Tool | Coordination mechanism | What to borrow / avoid |
|---|---|---|
| **Backlog.md** | Markdown tasks in git; agents claim a task and "lock the file to prevent concurrent edit conflicts"; acceptance criteria + Definition-of-Done review gates. | Borrow: files-in-git, human review gates (North's draft state already is one). Avoid: heavier per-task schema (DoD checklists) — North deliberately keeps the body freeform. |
| **kanban-md** | Frontmatter `claim` field + `pick --claim` + `agent-name` + `require_claim` + claim **timeout expiry** + `watch --json`. | Avoid: the roadmap already flags this cluster's flaws — frontmatter claims aren't atomic without a lock protocol, and **timeout expiry can put two agents on one task**. This is the anti-pattern. |
| **tick-md** | File lock on claim; every claim/work/complete is a git commit (audit trail); dependency auto-unblock. | Borrow: claim-as-commit audit trail maps onto North's `auto_commit`. Avoid: nothing new — it's Backlog.md's model. |
| **Worktree "shared task doc" pattern** (the dominant 2026 practice) | Each agent gets an isolated git *worktree* for code; **all agents read/write one shared task list** to avoid duplicate effort. | This is the key external finding — see §5. It directly tensions with North's *committed, therefore per-worktree-isolated* board. |

The common thread in the good implementations: **a real lock around the
select-and-claim step**, ownership recorded as an ordinary field, and a git
trail. The common failure: **timeout-based claim expiry**, which trades one
race (two agents pick at once) for a worse one (a slow-but-alive agent's
claim expires and a second agent is handed the same task). North should take
the lock and skip the timeout.

Sources:
[Backlog.md](https://github.com/MrLesk/Backlog.md),
[tick-md coordination](https://purplehorizons.io/blog/tick-md-multi-agent-coordination-markdown),
[git worktrees for parallel agents](https://www.mindstudio.ai/blog/parallel-ai-coding-agents-git-worktrees),
[the code agent orchestra](https://addyosmani.com/blog/code-agent-orchestra/).

---

## 4. Do we need a "claim" system? No.

A dedicated claim system (kanban-md style) adds: a `claim`/lease frontmatter
key, a claimant identity separate from `assignee`, a timeout/expiry policy,
`require_claim` config, and reaper logic. That is a lot of new surface, and
its hardest parts (atomicity, expiry, crash recovery) are exactly where
kanban-md gets it wrong.

North already owns two fields that *together* express a claim:

- **`assignee`** (free-form) → **who** possesses the task ("intent/possession").
- **`status: in_progress`** → **that** it is possessed ("this is being worked").

A task that is `in_progress` **and** assigned to `A` **is** A's claim. No new
field is needed. The only thing missing is doing the select + set-status +
set-assignee **atomically under the existing `.lock`**. That is one new
operation in `internal/tasks`, not a subsystem.

Crash recovery falls out for free and *without* the timeout trap: a crashed
agent leaves a task `in_progress` assigned to it. Nothing auto-reassigns it
(that is the kanban-md bug). A human or orchestrator resets it explicitly
(`north task move X ready`) — see §7. Possession is a fact on disk, never a
lease that silently expires.

**Recommendation: no claim data model. Reuse `assignee` + `in_progress`,
made atomic by the lock.** This is the minimal-surface answer and it is
strictly more correct than the claim-cluster it replaces.

---

## 5. Topologies — the real crux

The user wants all topologies theorised. The decisive variable is **whether
agents share one physical `north/` directory or each have their own copy**,
because the `.lock` (and thus atomic `take`) only works *within a single
physical directory on one filesystem*.

### 5.1 Approach 1 — same directory, same branch (the naïve default)

All agents run in one working tree; they share the same `north/` files **and**
the same source-code working tree.

- **Board coordination: works perfectly.** One physical `north/`, one
  `.lock`, atomic `take` fully closes the TOCTOU window. This is the *only*
  topology where live claim coordination Just Works.
- **Code coordination: chaos.** 5–10 agents editing the *same* source tree
  and git index simultaneously clobber each other's edits, staging, and
  branch state. North coordinates the *board*; it cannot coordinate the
  *code*. This is out of North's scope but must be stated loudly, because it
  is the topology an inexperienced user will reach for first.
- **Verdict:** fine for board-only or read-mostly/planning agents, or a
  human + one working agent. **Not** safe for many agents concurrently
  writing code. North should make the board safe here and the docs should
  warn that the *code* isn't.

### 5.2 Approach 2 — git worktrees (recommended for real parallel code work)

Each agent gets its own `git worktree add` checkout on its own branch,
sharing one `.git`. This is the dominant 2026 practice for parallel agents
and the one the user is happy to require.

- **Code coordination: excellent** — isolated working trees, no cross-agent
  file clobbering, clean per-branch diffs to review and merge.
- **Board coordination: this is the crux.** Because `north/` is **committed
  into the repo**, each worktree gets its *own physical copy* of
  `north/tasks/*.md` **and its own `north/.lock`**. A `take` in worktree-A
  marks `12` in_progress *on branch-A only*; worktree-B's board still shows
  `12` ready. **Live cross-worktree claiming is impossible with committed
  files** — the lock is local to a filesystem directory, and each worktree
  is a different directory with a different copy.
- Coordination therefore becomes **merge-time**: two agents can both `take`
  12 (on different branches) and both `create` a task that mints the same
  id. On merge you get conflicting edits to `12-*.md` and/or duplicate ids —
  the latter already handled by `north doctor --fix`, the former is an
  ordinary git conflict ("git is yours").
- **Verdict:** the right topology for the *work*, but North's committed board
  cannot provide live claim coordination across worktrees. Two honest ways
  to reconcile — §5.5.

### 5.3 Approach 3 — separate clones/repos

Each agent in a full clone, reconciled by push/pull/merge. North never
touches remotes, so this is Approach 2's merge-time story with more latency
and more merge surface, and no shared filesystem at all. The user already
called this overkill. **Rejected as a recommended path** — it adds nothing
over worktrees except distance.

### 5.4 Approach 4 — one orchestrator, N workers

A parent process assigns disjoint work and spawns workers. This is the
user's separate side project and is explicitly *not required* for North to
work. It is, however, the cleanest way to get parallel code work *and* zero
board contention: the orchestrator hands each worker a specific task id (or
a disjoint filter/label), so workers never race to pick. North's job is
simply to expose the primitives (`next`, `take`, `--assignee` filter) that
such an orchestrator drives. **North should not build the orchestrator, but
its command surface should be orchestrator-friendly** (scriptable, `--json`,
exit codes — all already true).

### 5.5 Reconciling worktrees with a shared board

The external consensus is: **isolated worktrees for code + one shared task
list for coordination.** North's board is committed, so it is *not* shared
across worktrees by default. Two ways to get the shared board back:

- **(A) Merge-time coordination + up-front partitioning (no new code).**
  The human/orchestrator gives each agent a *disjoint* slice up front (by
  label, assignee, or an explicit id list) so agents never contend for the
  same task. Each agent `take`s only within its slice; branches merge back;
  `doctor` heals duplicate ids. This honours "git is yours" and needs
  nothing new beyond `take` + the existing `--assignee`/`--label` filters.
  **This is the recommended default for worktrees.**

- **(B) One shared board, isolated code (needs one enabler).** Keep a single
  physical `north/` and have every agent's `take`/`next`/`move` operate
  against *that* directory, while each agent's *code* lives in its own
  worktree. Live locking and atomic `take` then work across all agents again.
  The blocker: North finds the board only by **walking up from cwd** — an
  agent in its worktree finds *its* board, not the shared one. Making (B)
  ergonomic needs a way to point North at a board outside the walk-up: a
  read-only **`NORTH_DIR` env var** (preferred over a `--board` flag; the
  roadmap rejected the general `-C <dir>`, but a narrow env override is a much
  smaller concession, and env fits the "set once per tmux pane" workflow).

#### 5.5.1 `NORTH_DIR` walkthrough (discussed 2026-07-10)

Concrete setup: repo at `~/proj` on `main`, board committed at `~/proj/north/`.
Worktrees:

```bash
git worktree add ../proj-a -b agent-a
git worktree add ../proj-b -b agent-b
# ~/proj    (main)     → ~/proj/north/     (board copy on main)
# ~/proj-a  (agent-a)  → ~/proj-a/north/   (separate physical copy)
# ~/proj-b  (agent-b)  → ~/proj-b/north/   (separate physical copy)
```

*Default (no `NORTH_DIR`):* agent in `~/proj-a` runs `north take` → walks up →
finds `~/proj-a/north` → claims on branch `agent-a` only; agent-b never sees
it → merge-time coordination.

*With `NORTH_DIR`:* each pane exports one shared path; `north` skips walk-up
and uses it, so all agents hit the **same physical `.lock`/files on one
filesystem** → live atomic `take` across all agents. Three flavors of *what*
to point at, and they differ substantially — which is the "board outside your
working dir feels different" concern, made precise:

- **(a) main checkout's board** — `export NORTH_DIR=~/proj/north`. Live shared
  board, but `~/proj`'s tree churns with uncommitted claim writes (its
  `git status` is always dirty) and board edits land on `main` while code
  edits are on `agent-*` branches. Board/code decoupled onto different
  branches.
- **(b) standalone sidecar** — `export NORTH_DIR=~/proj-board/north` (a plain
  dir, not a checkout). Cleanest isolation, but the board is **no longer
  committed inside the repo** — a sidecar file tree. Contradicts
  "files-in-the-repo are the source of truth." Avoid.
- **(c) dedicated board *worktree*** — `git worktree add ../board -b board`
  then `export NORTH_DIR=~/board/north`. Board lives in its **own** worktree
  on its **own** branch: shared live (same inode + `.lock`) ✓, **still
  git-committed inside the repo** on the `board` branch ✓, decoupled from
  every code worktree so nothing churns their trees ✓, and `auto_commit`
  gives one clean claim-audit timeline on `board`. Only oddity: the board is
  on a different branch than the code — arguably *correct*, since the board is
  orthogonal to code. **This is the flavor to steer docs toward if we adopt
  `NORTH_DIR`.**

Costs of `NORTH_DIR` (real but bounded): a second board-discovery path that
`doctor`, error messages, and the skill must acknowledge; and the footgun of a
stale `NORTH_DIR` targeting the wrong board — mitigated by having `north`
print the resolved board path when the env var is set.

**Standing recommendation (not yet decided):** keep the default worktree story
as (A) merge-time + partition, preserving the principle at zero surface;
offer `NORTH_DIR` as a documented **opt-in** power-user/orchestrator hatch and
steer it to flavor (c). Then 99% of users never touch it and the "board in the
repo" identity holds; the swarm crowd gets live coordination for one line.
**Decision deferred — see §10.**

**Summary of the crux:** atomic `take` is a complete solution *within a
shared directory* (Approach 1, Approach 4, or Approach 2-variant-B). Across
isolated worktrees (Approach 2-variant-A) coordination is necessarily
merge-time + partition-up-front. There is no way around this without a
shared filesystem location or a daemon/network — and the latter two are
constraint violations. We should be explicit about that boundary rather than
pretend `take` coordinates worktrees it cannot see.

---

## 6. Recommended primitive: `north next` + `north take`

Two commands, exactly as the user framed it. Both are thin sugar over
existing selection + mutation logic; only the *atomic* one needs new code.

### 6.1 `north next` — peek (read-only)

Show the single most-workable task without touching anything.

```bash
north next [--assignee A] [--label L] [--json | --plain]
```

- Selection = the existing "workable" definition: `state active`, `status
  ready`, dependencies **met** (reuses `snap.UnmetDeps`), **unassigned by
  default** (so it surfaces genuinely up-for-grabs work).
- Ordering = deterministic: lowest id first (oldest ready work), matching
  the existing default sort. Deterministic order is what makes two agents'
  `next` agree on "the top" — and what makes `take` hand out *different*
  tasks in sequence.
- Pure read. Safe to call anytime, by humans ("what's up next?") or by an
  agent that wants to decide before committing.
- Essentially `task list --status ready --deps met --sort id` limited to the
  top row — could even be documented as sugar for that.

### 6.2 `north take` — atomic claim

Select the top workable task **and** claim it, all under one `.lock` hold.

```bash
north take --assignee A [--label L] [--json | --plain]
```

Under a single `board.Lock()` acquisition:
1. Load the snapshot.
2. Pick the first workable task (same selection/order as `next`).
3. Set `status = in_progress` **and** `assignee = A` in one write.
4. Return the task (or a clean "nothing workable" result — see §10).

Because select + claim happen inside one lock hold, two concurrent `take`s
get **different** tasks: the second sees the first already `in_progress`
(and assigned) and moves to the next workable row. **This is the whole point
— it closes the §2 TOCTOU window that two separate calls cannot.**

Naming: `take` is recommended precisely because it is *not* "claim" — it
signals we did **not** build the claim cluster. `north next --take` is a
viable single-command alternative but two distinct verbs read better in the
skill and in an agent's muscle memory. (Top-level `north next`/`north take`
mirrors `north board`/`north cleanup`; `north task next`/`north task take`
is the alternative if we prefer to keep everything under `task`.)

### 6.3 Why this is the minimal surface

New surface added:
- Two CLI commands (`next`, `take`).
- One new `internal/tasks` function (`TakeNext`) — the only piece with real
  concurrency logic — plus a trivial `Next` (read-only selection).
- Skill: a few lines swapping the "list → move" loop for "`take`", and a note
  on `next`.

New surface **explicitly not** added (and why):
- No `claim`/lease frontmatter field (assignee + in_progress already are it).
- No timeout/expiry/reaper (the kanban-md double-assign trap; §4).
- No `require_claim`/`agent-name` config (assignee is the identity; §6.4).
- No `watch`/daemon (the board is plain files — any watcher works; a daemon
  violates the no-daemon constraint).
- No `watch`/daemon (repeated for emphasis — plain files, any watcher).

Note (revised 2026-07-10): the earlier roadmap rejected a *WIP-limit config*.
The multi-agent story reopens a **narrow** version of it — a per-assignee
`in_progress` cap enforced at `take` time (`max_wip`, §7) — which the user
has accepted as a board config key. This is not the rejected per-status
column WIP display; it's a claim-time guard against one agent grabbing
multiple tasks.

---

## 6.4 Identity — where does `--assignee` come from?

`take` needs a stable caller identity to write into `assignee`. Options:

- **Required `--assignee` flag** — explicit, zero magic, but the agent must
  know its own name.
- **`NORTH_AGENT` env var as the default for `--assignee`** (recommended
  convenience) — each tmux pane exports its identity once
  (`export NORTH_AGENT=claude-a`), and `take` uses it unless `--assignee`
  overrides. No board state, no config; just a per-process default. This
  pairs naturally with the per-pane, per-worktree setup users already do.
- Auto-deriving from `$USER`/model name — rejected: ambiguous when 5 panes
  are the same user/model, which is exactly our case.

Recommendation: `take` uses `--assignee`, defaulting to `NORTH_AGENT` when
set, and **errors if neither is present** (a claim with no claimant is
meaningless). This is the same `NORTH_DIR`-style env pattern as §5.5(B),
which argues for treating "env overrides for agent ergonomics" as one small,
coherent addition rather than two ad hoc ones.

---

## 7. Crash recovery & stale work (no timeouts)

A crashed/killed agent leaves a task `in_progress` assigned to it. We
deliberately do **not** auto-reclaim (the kanban-md expiry bug). Instead:

- **Resume is already supported**: an agent restarting re-finds its own work
  with `north task list --assignee A --status in_progress` — no new surface.
- **WIP guard on `take` (board config — decided 2026-07-10)**: `take` refuses
  (or returns the existing task) when the caller's assignee already holds
  `in_progress` work, so a double-invoked agent doesn't grab two. Per the
  user's decision this is a **board config key**, not a hardcoded `1` — e.g.
  `max_wip` (per-assignee count of `in_progress` tasks; `0` = unlimited).
  Open: the **default value** (1? 0?) and the **scope** — gate only `take`
  (keeps `move X in_progress` a pure freeform primitive) vs. gate any
  transition into `in_progress` (more consistent, constrains manual moves).
  Both still open (§10).
- **Surfacing stale work (read-only)**: optionally `north task list
  --status in_progress` (already possible) is the human's reaper view; a
  human/orchestrator resets a truly-dead task with `north task move X ready`.
  We could add a `--stale` age filter later, but **never** an automatic
  reset. Keep reclamation a human/orchestrator decision.

This keeps possession a durable fact, never a silently-expiring lease.

---

## 8. Interaction with existing design (all consistent)

- **Draft gate preserved.** New tasks still land in `drafts/` (human gate);
  `next`/`take` only consider `active` + `ready`. A human or orchestrator
  promotes tasks to active; agents take from active. The two-axis model is
  untouched and the human gate becomes the natural "release work to the
  swarm" control.
- **Dependencies reused verbatim.** `take` handing out only deps-met tasks
  is just the existing `--deps met` resolution — no new dep logic. On strict
  boards, `move done` with unmet deps is already refused; consistent.
- **`auto_commit` gives the tick-md-style audit trail for free.** With
  `auto_commit: true`, each `take` is a local commit
  (`north: 12 → in_progress`) — who took what, when, in git history, no new
  code. Note: `take` writes both status and assignee, so the commit message
  could read `north: take 12 (A)` for clarity.
- **`doctor` still owns post-merge repair** — duplicate ids from concurrent
  `create` across branches, dangling deps. Unchanged.

---

## 9. Small real gaps to fix alongside (found while reviewing)

- **`north/.lock` is not gitignored — DECIDED to fix (2026-07-10).** It is
  created in the committed `north/` dir, so a mutation in flight (or a crashed
  holder's leftover lock) can show up in `git status` and even get committed.
  `init` will scaffold a `north/.gitignore` (`.lock`, `*.tmp`), written only
  when missing (like the other board-owned files), and `north doctor` will
  warn / `--fix` restore it — the same treatment `.gitattributes` already
  gets. Matters more once many agents are minting/stealing locks. (The `.tmp`
  files from atomic writes are renamed away immediately, but belt-and-braces.)
- **`SetStatus` early-return hides no-op claims.** `move X in_progress`
  returning success when X is *already* in_progress is fine for `move`, but
  it's precisely why the two-call "claim" is unsafe. `take` sidesteps this by
  selecting an *unassigned ready* task under the lock; worth a comment so a
  future reader doesn't "optimise" `take` back into `move`.
- **Stale-lock window vs. slow agents.** `staleAfter = 10s` assumes a
  mutation is sub-10s. `take`'s work (snapshot load + one write) is
  well under that, so this is fine — but if `take` ever grows heavier, revisit.
  Flagging so it's a conscious constant, not an accident, in the multi-agent
  era.

---

## 10. Decision log (as of 2026-07-10)

### Decided

- **No claim data model.** Reuse `assignee` + `status: in_progress` as the
  claim; atomicity comes from the existing `.lock`, not a lease/timeout. (§4)
- **Two top-level commands: `north next` (peek) + `north take` (atomic
  claim).** Placement is top-level, mirroring `north board`/`north cleanup`,
  not under the `task` namespace. (§6)
- **Empty-result contract: exit 0 + explicit `{"task": null}`** (plain: an
  empty/"no workable task" line). "No work" is a normal outcome an agent
  polls on, not an error. Applies to both `next` and `take`. (§6)
- **WIP guard is a board config key, not hardcoded.** A per-assignee
  `in_progress` cap (e.g. `max_wip`) gates `take`. (§7) — value/scope still
  open, below.
- **`north/.lock` (and `*.tmp`) get gitignored** via a scaffolded
  `north/.gitignore`, with `doctor` warn/`--fix`, mirroring `.gitattributes`.
  (§9)

### Closed 2026-07-12 (see [050_multi-agent-next-take.md](050_multi-agent-next-take.md))

1. **`NORTH_DIR` — rejected.** The user's multi-project test settled it: a
   shell-scoped env var naming an absolute board path silently mutates the
   wrong project's board when a shell is reused across projects. Walk-up
   stays the only discovery path; worktrees coordinate merge-time with
   up-front partitioning. (A future *project-scoped* redirect file inside the
   worktree would avoid the footgun — noted, not designed.)
2. **`max_wip` default `0`** (unlimited; the guard is opt-in).
3. **`max_wip` gates only `north take`** — `move` stays freeform.
4. **`NORTH_AGENT` env default — accepted.**

Implemented in plan 050. The list below is preserved as it stood before the
decisions:

### Open — user will decide later (historical)

1. **`NORTH_DIR` board-location escape hatch (biggest fork).** Add a
   read-only env override so worktree agents can share one physical board and
   keep live atomic `take`, or not? Full walkthrough + three flavors (a/b/c)
   in §5.5.1. Standing recommendation: **opt-in only, docs steer to flavor
   (c) — the dedicated board worktree**; keep the default worktree story as
   merge-time + partition (§5.5-A). Alternatives on the table: add it with no
   steer; or defer entirely (walk-up only, worktrees = merge-time). The
   "board outside your working dir is substantially different" concern is
   real and is exactly what §5.5.1 makes precise — this is why it's deferred
   for a considered call.
2. **`max_wip` default value.** `1` (one in-flight task per agent — safest
   default, matches the double-invoke guard intent) vs. `0`/unlimited
   (opt-in, least surprising for existing single-agent users). (§7)
3. **`max_wip` scope.** Gate **only `north take`** (keeps `move X
   in_progress` a pure freeform primitive; recommended) vs. gate **any**
   transition into `in_progress` (more consistent, but constrains manual/human
   moves). (§7)
4. **`NORTH_AGENT` env default for `--assignee` on `take`.** Accept the env
   default (each pane `export NORTH_AGENT=…` once; `--assignee` overrides;
   error if neither set — the ergonomic win for the tmux workflow) vs. require
   an explicit `--assignee` on every `take`. Full explanation in §6.4. (§6.4)

---

## 11. Recommendation in one paragraph

Do **not** build a claim system. Add two commands — `north next` (read-only
peek at the top workable task) and `north take` (atomically select-and-claim
the top workable task under the existing `.lock`, setting `status=in_progress`
+ `assignee`). This closes the only real concurrency gap (the list→move
TOCTOU race) with one new `internal/tasks` function and reuses `assignee` +
`in_progress` as the claim, `--deps met` for workability, the draft state as
the human release-gate, and `auto_commit` as the audit trail. Recommend git
worktrees for parallel *code* work, and resolve the "worktrees isolate the
committed board" crux by either (default) partitioning work up front +
merge-time reconciliation, or (better, if we accept it) a small `NORTH_DIR`
env override that lets all worktree agents share one physical board and thus
keep live atomic claiming. Ship the `north/.gitignore` fix alongside. No
daemon, no network, no timeouts, no new frontmatter — the minimal surface
that actually supports 5–10 agents.

---

## Change history

- [2026-07-10] Initial review written (topology analysis, prior-art survey,
  `next`/`take` recommendation, open questions).
- [2026-07-10] Walked §10 with the user. **Decided:** no claim model;
  top-level `north next`/`north take`; empty-result = exit 0 + `{"task":
  null}`; WIP guard becomes a board config key (`max_wip`); scaffold
  `north/.gitignore` for `.lock`/`*.tmp` (doctor warn/fix). Added the full
  `NORTH_DIR` walkthrough with three flavors (§5.5.1) and steered the
  standing recommendation to the dedicated board-worktree pattern (flavor c),
  opt-in only. **Still open (user deciding later):** whether to add
  `NORTH_DIR` at all; `max_wip` default value and scope; `NORTH_AGENT`
  assignee default. Promote to implementation plan (050) once these four
  close.
- [2026-07-12] All four open questions closed (§10): NORTH_DIR rejected
  (multi-project footgun), max_wip default 0 gating only `take`, NORTH_AGENT
  accepted. Implemented via
  [050_multi-agent-next-take.md](050_multi-agent-next-take.md).
