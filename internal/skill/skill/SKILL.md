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

A task has two independent properties. Both are **freeform** — any value can
change to any other value in a single command; there is no required path.

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
north task list --status ready --plain    # what is available?
north task view 12 --json                 # read the full brief (body included)
north task move 12 in_progress            # claim it
# …do the work…
north task edit 12 --append-body "Results: implemented X, tests pass."
north task move 12 done                   # or: failed / blocked (say why in the body)
```

When you finish, set `done`. If the work cannot succeed, set `failed`. If you
are stuck on something outside your control, set `blocked`. In every non-done
case, append an explanation to the body first.

## Commands

Create and lifecycle:

```bash
north task create "<title>" [--agent A] [--labels a,b] [--depends-on 3,4] [--body "..." | --body-file F]
north task state <id> <draft|active|archive>   # move between lifecycle states (any → any)
north task delete <id> -y                      # remove permanently (always pass -y)
```

Status (any → any, in any state):

```bash
north task move <id> <status>    # ready | in_progress | blocked | done | failed
```

Status can be set in any state, but it only shows on the board while the task
is active (a note is printed on stderr otherwise) — normally set state to
active first, then move.

Edit fields and body:

```bash
north task edit <id> [--title T] [--agent A] [--labels a,b] [--depends-on 3,4]
north task edit <id> --append-body "note"      # append to the body (safe for logging progress)
north task edit <id> --body "..." | --body-file F   # REPLACE the whole body
```

**`--body` and `--body-file` replace the entire body — prior content is lost.
Prefer `--append-body` for progress notes and results.** For multi-line text,
use `--body-file` (or `--body-file -` to read stdin) rather than trying to
escape newlines in `--body`. `--labels`/`--depends-on` replace the full list
(pass an empty value to clear).

Query and maintenance:

```bash
north task list [--state draft|active|archive|all] [--status S] [--search TEXT] [--label L] [--plain | --json]
north task view <id> [--plain | --json]
north board [--plain | --json]        # counts per status (active) + draft/archive tally
north cleanup [--older-than DAYS]     # archive active 'done' tasks
north doctor [--fix]                  # board integrity check (duplicates, cycles, bad files)
north config get|set|list             # board settings (e.g. auto_commit)
```

## Output and errors

- Every task/board command supports `--plain` (tab-separated) and `--json`.
  The default is human-formatted; always pass one of the two.
- On failure: exit code is non-zero and `error: <message>` is printed to
  stderr. With `--json`, the error is emitted instead as
  `{"error":{"code":"not_found|conflict|invalid|internal","message":"…"}}`.
- List/board `--json` payloads include a `"warnings"` array naming any task
  files that could not be parsed; in human/plain modes warnings go to stderr.

## Rules for agents

- Run `north board` when you need an overview or are unsure a board exists
  (if it reports no board, run `north init`); skip it when the request already
  names a task.
- A freshly created task is a **draft** — `north task state <id> active` puts
  it on the board before `move` can change its status.
- Check a task's `depends_on` before starting it: dependencies that are not
  `done` are a signal the task may not be ready (north does not enforce this).
- Record plans, progress, blockers, and results in the task **body**
  (prefer `--append-body`); north does not impose body structure.
- Drive the board through these commands rather than editing task files by
  hand, so ids, status, and timestamps stay consistent.
- Never use `north tui` — it needs a real TTY and produces no machine-readable
  output.
