# 5. Configuration

## `north/config.yml`
Created by `north init`. It is both the **board-discovery marker** (North walks
up looking for it) and the home for per-board settings.

```yaml
auto_commit: false   # commit each board change locally (never pushes)
```

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
configurable here is future work).

## State
North keeps no global state — there is no `~/.north`, no daemon, and no
environment configuration. Everything lives in the repo under `north/`.
