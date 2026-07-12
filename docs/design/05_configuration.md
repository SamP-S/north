# 5. Configuration

## `north/config.yml`
Created by `north init`. It is both the **board-discovery marker** (North walks
up looking for it) and the home for per-board settings.

```yaml
version: 1                   # board format stamp (read-only; written by init)
auto_commit: false           # commit each board change locally (never pushes)
deps_enforcement: validated  # depends_on enforcement: hint | validated | strict
max_wip: 0                   # per-assignee in_progress cap enforced by `take` (0 = unlimited)
```

- `version` — the board format this North wrote. Loading a board with a
  newer stamp is refused ("created by a newer north") instead of misread; a
  missing key means a pre-stamp board and is treated as version 1. The key is
  read-only: it shows in `config list`/`get`, and `set` refuses it. There is
  no migration machinery until a `version: 2` exists.
- `deps_enforcement` — how strictly `depends_on` is enforced on writes:
  `hint` (warn only, forward refs allowed), `validated` (default — dangling
  ids/self-refs/cycles refused, delete heals dependents, workflow order
  warns), `strict` (validated + `move done`/`in_progress` with unmet deps
  refused). Writes-only: changing the level never touches stored tasks. See
  the event matrix in [02_board-data-model.md](02_board-data-model.md).
- `max_wip` — a non-negative integer capping how many **active
  `in_progress`** tasks one assignee may hold, enforced **only by
  `north take`** (a `conflict` naming the held ids). `0` (default) disables
  the guard. Assignees compare case-insensitively ("Claude-A" and "claude-a"
  are one agent), though the stored value keeps its casing. `task move <id> in_progress` is deliberately not gated — it
  stays a pure freeform primitive; the cap is a claim-time guard (e.g. a
  double-invoked agent grabbing two tasks), not a workflow rule.
- `auto_commit` — when `true`, North shells out to the system `git` to
  `add` + `commit` the changed `north/…` files after each mutation; when
  `false` (default) it only writes/moves files and leaves git to you. Commits
  work in linked worktrees and fall back to a `north <north@localhost>`
  identity when the user has none configured. North never pushes or pulls.

Read and write settings with the CLI rather than editing by hand:

```bash
north config list
north config get auto_commit
north config set auto_commit true
```

A malformed `config.yml` is a hard error (`invalid`), not a silent fallback —
a YAML typo must not silently change behaviour.

States, statuses, and the id scheme are hardcoded for now (making them
configurable here is future work). The task body template is deliberately a
file (`north/task-template.md`), not a config setting.

## User-level config: `~/.north/config.yml`
Board config (above) is policy, committed and shared. Some preferences —
today, the TUI's color theme — are personal and belong to the user, not the
repo, so they live separately in **`~/.north/config.yml`**. North never
rewrites or re-adds keys to a file the user owns; this file is edited by
hand, not through `north config` (see below).

```yaml
# north user settings (per-user, not per-board)
tui:
  # theme: default | saturated | high-contrast
  theme: default
```

- `tui.theme` — one of three strict lowercase presets (no aliases):
  `default` (inherit the terminal's own ANSI 0–15 palette — the terminal
  theme is the theme), `saturated` (a fixed vivid truecolor palette,
  terminal-independent), `high-contrast` (ANSI brights only, no dim greys).
- On `north tui` startup, the file is scaffolded with the commented template
  above if it doesn't exist yet (this is the discoverability story — users
  find the valid values in the file itself), and is never touched again once
  present, even if it's missing the `tui:` block or the `theme` key (reads
  as default silently).
- **Never blocks the TUI**: a missing/unwritable scaffold, an unreadable or
  malformed file, or an unknown theme name all fall back to the `default`
  theme with a yellow status-bar warning instead of failing to start.
- Non-goals: no per-slot color configuration (only the three built-in
  presets), no `NORTH_THEME` environment variable override (one config
  source only), and `north config get/set/list` stays board-scoped to
  `north/config.yml` — it does not read or write this file.

## Environment: `NORTH_AGENT`
The one environment variable North reads: the default for `take --assignee`
(the flag overrides; neither set is an `invalid` error). It is a per-process
identity convenience for the multi-agent workflow — each agent's shell/pane
does `export NORTH_AGENT=claude-a` once — not configuration: it selects no
board, changes no behaviour, and holds no state. Deliberately **not** a
board-location override (a shell-scoped path variable would be a
cross-project footgun; board discovery stays walk-up only).

## State
North keeps no global board state — there is no daemon, and no environment
variable configures behaviour (`NORTH_AGENT` above is an identity default,
not configuration). Board data lives entirely in the repo under `north/`;
the one exception is the user-level TUI preference file above, which is
per-machine and deliberately outside the repo.
