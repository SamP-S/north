---
name: north
description: >
  Manage project tasks with north, an in-repo Markdown task board CLI.
  Use when the user mentions tasks, the board, backlog, todos, project
  tracking, or wants to create, list, view, move, promote, archive, or
  otherwise manage work items. Also use for status updates and triage.
allowed-tools:
  - Bash(north *)
---

# north

`north` is an in-repo Markdown task board. Each task is a `.md` file with YAML
frontmatter, stored under a `north/` directory in the repo. The board is found
by walking up from the working directory (like `.git`), so commands work from
any subdirectory. There is no server — `north` operates directly on files.

Task ids are bare numbers (`1`, `12`), unique across the whole board.

## Two axes: state and status

A task has two independent properties. Each takes one value from a fixed set,
but movement is free — any value can change to any other value in a single
command; there is no required path.

- **State** = where the task is in its lifecycle (which folder it lives in):
  - `draft` — captured but not yet on the active board (a human gate).
  - `active` — on the board, being worked.
  - `archive` — off the board, kept for history.
- **Status** = the workflow column, only meaningful (and only changeable)
  while the task is **active**:
  - `ready` — available to be picked up.
  - `in_progress` — actively being worked.
  - `done` — finished successfully.
  - `failed` — attempted but did not work out; body should say why.
  - `blocked` — cannot proceed (waiting on a dependency, decision, or human);
    body should say what unblocks it.

New tasks start as a **draft** with status `ready`. Move a task to `active`
before changing its status.

## Typical loop

```bash
north take --json                         # atomically claim the next workable task
north task view 12 --json                 # read the full brief (body included)
# …do the work…
north task edit 12 --append-body "Results: implemented X, tests pass."
north task move 12 done                   # or: failed / blocked (say why in the body)
```

`north take` selects the next workable task (active, `ready`, unassigned,
dependencies met, lowest id) and claims it — `in_progress` + assignee — in
one atomic step under the board lock, so concurrent agents always get
*different* tasks. **Always claim with `take`, never with `list` + `move`:**
the two-call version races against other agents (both can pick the same
task). `take` needs an identity for the assignee: `--assignee A`, or the
`NORTH_AGENT` environment variable (ask the user to `export NORTH_AGENT=…`
per agent if unset). `{"task": null}` with exit 0 means nothing is workable —
stop or wait; it is not an error. A `conflict` mentioning `max_wip` means you
already hold your limit of in-progress tasks — finish those first (find them
with `north task list --assignee <you> --status in_progress`).

To preview without claiming, `north next --json` shows the same pick
read-only. Use `north take --label L` to claim only from a labelled slice
when the user has partitioned work by label.

When you finish, set `done`. If the work cannot succeed, set `failed`. If you
are stuck on something outside your control, set `blocked`. In every non-done
case, append an explanation to the body first.

## Commands

Create and lifecycle:

```bash
north task create "<title>" [--assignee A] [--labels a,b] [--depends-on 3,4] [--body "..." | --body-file F]
north task state <ids> <draft|active|archive>  # move between lifecycle states (any → any)
north task delete <ids> -y                     # remove permanently (always pass -y)
```

A bodyless `create` fills the body from `north/task-template.md` (Summary /
Acceptance Criteria / Notes / Changes / Comments by default — the user may
have edited or deleted it). Follow the section layout it gives you.

Status (any → any, in any state):

```bash
north task move <ids> <status>   # ready | in_progress | blocked | done | failed
```

`move`, `state`, and `delete` take one id or a comma-delimited batch
(`north task move 2,3,4 done`): ids are deduplicated, every id is attempted
(continue-on-error) with a per-id report, and any failure makes the exit
non-zero.

Status can be set in any state, but it only shows on the board while the task
is active (a note is printed on stderr otherwise) — normally set state to
active first, then move.

Edit fields and body:

```bash
north task edit <id> [--title T] [--assignee A] [--labels a,b] [--depends-on 3,4]
north task edit <id> --append-body "note"      # append to the body (safe for logging progress)
north task edit <id> --body "..." | --body-file F   # REPLACE the whole body
```

**`--body` and `--body-file` replace the entire body — prior content is lost.
Prefer `--append-body` for progress notes and results.** For multi-line text,
use `--body-file` (or `--body-file -` to read stdin) rather than trying to
escape newlines in `--body`. `--labels`/`--depends-on` replace the full list
(pass an empty value to clear).

Picking work (multi-agent safe):

```bash
north next [--label L] [--plain | --json]                 # peek at the next workable task (read-only)
north take [--assignee A] [--label L] [--plain | --json]  # claim it atomically (assignee falls back to $NORTH_AGENT)
```

Query and maintenance:

```bash
north task list [--state draft|active|archive|all] [--status S] [--assignee A] [--deps met|unmet] [--search TEXT] [--label L] [--sort id|updated|title|assignee] [--reverse] [--plain | --json]
north task view <id> [--plain | --json]
north board [--plain | --json]        # counts per status (active) + draft/archive tally
north cleanup [--older-than DAYS]     # archive active 'done' tasks
north doctor [--fix]                  # board integrity check (duplicates, cycles, bad files)
north config get|set|list             # board settings (auto_commit, deps_enforcement, max_wip)
```

## Dependencies

A `depends_on` entry is **resolved** once that task is `done` or archived.
`--deps met` filters to workable tasks; `--deps unmet` to waiting ones.
Enforcement is per-board (`config get deps_enforcement`):

- `hint` — everything allowed; problems are stderr warnings.
- `validated` (default) — dangling ids, self-refs, and cycles are refused
  (`invalid`); deleting a task auto-removes it from dependents' `depends_on`;
  moving to done/in_progress with unmet deps succeeds with a warning.
- `strict` — additionally, `move <id> done|in_progress` is **refused**
  (`conflict`, exit 4) while deps are unmet. The error names the unmet ids:
  complete those tasks first, or `edit --depends-on` to drop stale links.
  Archiving is always allowed.

Successful mutations may carry advisory warnings: stderr lines in
plain/human modes, a `"warnings"` array in `--json` payloads.

## Output and errors

- Every task/board command supports `--plain` (tab-separated) and `--json`.
  The default is human-formatted; always pass one of the two.
- `task list --plain` columns: `id  state  status  assignee  labels  title`
  (tab-separated; assignee/labels empty when unset, labels comma-joined).
- Exit codes are one contract in every output mode: **0** success,
  **1** internal, **2** invalid/usage, **3** not_found, **4** conflict.
  A partially failed batch exits with the shared failure code (1 when mixed).
- On failure `error [<code>]: <message>` is printed to stderr. With `--json`,
  the error is emitted instead as
  `{"error":{"code":"not_found|conflict|invalid|internal","message":"…"}}`.
- List/board `--json` payloads include a `"warnings"` array naming any task
  files that could not be parsed; in human/plain modes warnings go to stderr.

## Rules for agents

- Run `north board` when you need an overview or are unsure a board exists
  (if it reports no board, run `north init`); skip it when the request already
  names a task.
- A freshly created task is a **draft** — `north task state <id> active` puts
  it on the board before `move` can change its status.
- **Claim work with `north take`, never `list` + `move`** — only `take` is
  atomic against other agents. On restart, resume your own work first:
  `north task list --assignee <you> --status in_progress`.
- Never reset or reassign another assignee's `in_progress` task on your own
  initiative — a task can look stale while its agent is still working. That
  call belongs to the user.
- Prefer `--deps met` when picking work manually (`take` already only offers
  deps-met tasks); on strict boards north refuses starting/finishing a task
  whose dependencies are unmet.
- Record plans, progress, blockers, and results in the task **body**
  (prefer `--append-body`); north does not impose body structure.
- Set `--assignee` to the identity working the task — a person ("john") or an
  agent ("claude:opus"); it is free-form and searchable.
- Drive the board through these commands rather than editing task files by
  hand, so ids, status, and timestamps stay consistent.
- Never use `north tui` — it needs a real TTY and produces no machine-readable
  output.
