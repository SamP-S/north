# 5. Configuration

## `north/config.yml`
Created by `north init`. It is both the **board-discovery marker** (North walks
up looking for it) and the home for per-board settings.

```yaml
auto_commit: false   # commit each board change locally (never pushes)
```

- `auto_commit` — when `true`, North runs `git add` + `git commit` of the changed
  `north/…` files after each mutation; when `false` (default) it only writes/moves
  files and leaves git to you. North never pushes or pulls.

States, statuses, and the `task-` id prefix are hardcoded for now (making them
configurable here is future work).

## State
North keeps no global state — there is no `~/.north`, no daemon, and no
environment configuration. Everything lives in the repo under `north/`.
