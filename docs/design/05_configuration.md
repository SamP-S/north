# 5. Configuration

## `north/config.yml`
Created by `north init`. It is both the **board-discovery marker** (North walks
up looking for it) and the home for per-board settings.

```yaml
version: 1                   # board format stamp (read-only; written by init)
auto_commit: false           # commit each board change locally (never pushes)
deps_enforcement: validated  # depends_on enforcement: hint | validated | strict
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

## State
North keeps no global state — there is no `~/.north`, no daemon, and no
environment configuration. Everything lives in the repo under `north/`.
