# 050 — Multi-agent support: `north next` + `north take`

**Status:** accepted, in implementation
**Date:** 2026-07-12
**Precursor:** [049_multi-agent-usage-review.md](049_multi-agent-usage-review.md)
(the design review; all analysis and rejected alternatives live there).

## Summary

Close the list→move TOCTOU race (049 §2) so 5–10 agents can share one board,
with the smallest possible new surface:

- **`north next`** — read-only peek at the top workable task.
- **`north take`** — atomically select **and** claim the top workable task
  under one `.lock` hold (sets `status=in_progress` + `assignee` in one write).
- **`max_wip`** board config key (default `0` = unlimited): per-assignee cap
  on active `in_progress` tasks, enforced **only by `take`** (`move` stays a
  pure freeform primitive).
- **`NORTH_AGENT`** env var: default for `take --assignee` (flag overrides;
  error if neither set).
- **`north/.gitignore`** scaffold (`.lock`, `*.tmp`) on `init`, with
  `doctor` warn/`--fix` — the 049 §9 hygiene fix.

No claim data model, no timeouts/expiry, no daemon, no new frontmatter keys.

### Decisions closing 049 §10 (final, 2026-07-12)

1. **`NORTH_DIR` — rejected.** A shell-scoped env var pointing at an absolute
   board path is a cross-project footgun: a stale export in a reused shell
   silently mutates the *wrong project's* board with no guard. Walk-up stays
   the only discovery path. Worktree coordination is merge-time +
   partitioning (049 §5.5-A). If live cross-worktree claiming is ever needed,
   the safer future shape is a *project-scoped* redirect file inside the
   worktree, not an env var — deferred, not designed here.
2. **`max_wip` default `0`** (unlimited; the guard is opt-in).
3. **`max_wip` scope: gates only `north take`.**
4. **`NORTH_AGENT` env default: accepted.**

## Behaviour spec

### Selection ("workable")

Shared by `next` and `take`: state `active`, status `ready`, **unassigned**
(`assignee` empty), all dependencies met (`snap.UnmetDeps` empty), and — when
`--label L` is given (repeatable) — carrying every requested label. Order:
lowest id first. Deterministic, matching the snapshot's active ordering.

### `north next [--label L] [--plain | --json]`

Pure read; never touches files or the lock. Prints the top workable task:
human = the task detail; `--plain` = plain task detail; `--json` =
`{"task": <task map>, "warnings": [...]}` (warnings = snapshot parse
warnings, as strings). **Empty result: exit 0**, human `No workable task.`
(stderr-free stdout note), plain prints nothing, json `{"task": null,
"warnings": [...]}`.

### `north take [--assignee A] [--label L] [--plain | --json]`

Assignee = `--assignee` flag, else `$NORTH_AGENT`, else `invalid` (exit 2):
a claim needs a claimant. Under **one** `board.Lock()` hold:

1. Load snapshot.
2. `max_wip` guard: if `max_wip > 0` and A already holds ≥ `max_wip` active
   `in_progress` tasks → `conflict` (exit 4) naming the held ids.
3. Pick the first workable task (same selection as `next`). None → success,
   empty contract (same as `next`).
4. Set `status=in_progress`, `assignee=A`, bump `updated_at`; single file
   write; auto-commit message `north: take <id> (<A>)`.

Because select+claim happen inside one lock hold, concurrent `take`s hand out
*different* tasks — this is the whole point (049 §2/§6.2). Selection only
offers deps-met tasks, so `take` is consistent with `strict` boards by
construction. Deliberately **not** implemented as `list` + `move`: `SetStatus`
early-returns on an unchanged status, which is exactly why the two-call claim
is unsafe (049 §9).

### Config

`north/config.yml` gains `max_wip: 0` (non-negative int; `0` = unlimited).
`north config list|get|set max_wip` supported; `set` validates ≥ 0.

### Init / doctor

`init` scaffolds `north/.gitignore` (content: `.lock`, `*.tmp`) when missing,
like `.gitattributes`. `doctor` reports a missing `north/.gitignore`
(kind `gitignore`) and `--fix` restores it.

## Files to modify

- `internal/board/board.go` — `GitignoreName` + content + `WriteGitignore`;
  `InitBoard` scaffold; `Config.MaxWIP` (`yaml:"max_wip"`) + load/validate.
- `internal/tasks/next.go` (new) — `Next`, `Take` (the one function with
  concurrency logic).
- `internal/tasks/doctor.go` — gitignore check/fix.
- `internal/cli/next.go` (new) — `north next`, `north take` commands.
- `internal/cli/root.go` — register both.
- `internal/cli/config.go` — `max_wip` key.
- `internal/skill/skill/SKILL.md` — take-based claim loop, `next`,
  `NORTH_AGENT`, multi-agent rules.
- `docs/design/03_cli.md`, `05_configuration.md`, `99_roadmap.md`,
  `README.md` — surface docs.
- `internal/tasks/next_test.go` (new), `internal/tasks/doctor_test.go`,
  `internal/cli/cli_test.go` — tests incl. a concurrent-take race test.
- `docs/plans/049_multi-agent-usage-review.md` — close the decision log.

## Todo

1. [x] Write this plan.
2. [x] board: `.gitignore` scaffold + `max_wip` config.
3. [x] tasks: `Next`/`Take` (+ doctor gitignore check).
4. [x] cli: `next`/`take` commands + `max_wip` config key.
5. [x] Tests: selection, atomicity (parallel takes get distinct tasks),
   `max_wip`, `NORTH_AGENT` resolution, empty contract, doctor/init gitignore.
6. [x] Skill + design docs + README updates.
7. [x] `make fmt vet test build` clean.
8. [x] Close 049 decision log; usage instructions delivered to user.

## Usage (user-facing summary)

```bash
# one-off per board (optional): cap agents to one task each
north config set max_wip 1

# per tmux pane (once):
export NORTH_AGENT=claude-a

# agent loop:
north take --json          # claims top ready+deps-met+unassigned task (or {"task": null})
# …work…
north task edit 12 --append-body "Results: …"
north task move 12 done

# human: what would an agent get next?
north next
```

Topology assumption: live coordination requires all agents to run against
**one physical checkout** (same `north/` dir, one `.lock`). Across git
worktrees each checkout has its own board copy — partition work up front
(labels/explicit ids) and reconcile at merge (`north doctor --fix` heals
duplicate ids). See 049 §5.

## Change history

- [2026-07-12] Plan written; NORTH_DIR rejected (multi-project footgun),
  max_wip default 0 / take-only, NORTH_AGENT accepted. Implementation started.
- [2026-07-12] Implementation complete: board config + gitignore scaffold,
  tasks.Next/Take, CLI `next`/`take`, `max_wip` config key, doctor gitignore
  check, skill + design docs + README updated, tests added (incl. 8-way
  concurrent take race test). `make fmt vet test build` clean.
- [2026-07-12] User decision: assignee comparisons are case-insensitive
  (`strings.EqualFold`), applied to the `max_wip` guard and the
  `task list --assignee` filter; stored casing is preserved.
- [2026-07-12] User decision: the README documents `next`/`take`/`max_wip`/
  `NORTH_AGENT` as ordinary commands and config only — it does **not**
  prescribe a multi-agent workflow (topologies, tmux setup, board-branch
  pattern). The user wants time before declaring a blessed workflow; the
  analysis stays in plan 049 and `docs/design/03_cli.md` (maintainer-facing)
  in the meantime.
