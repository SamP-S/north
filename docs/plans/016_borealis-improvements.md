# 016 — Borealis Improvements (CLI/API Repair + Write Commands)

## Summary

Make Borealis correct and usable standalone. Three groups, ordered: fix what
is broken today, expose missing API capability, then give the CLI full board
CRUD so the board can be managed without curl.

Hardening items (response models, richer `/api/status`, lifespan migration,
mutate/reload lock, resolver commit batching) are deferred to a later plan.
Status-transition validation is deliberately excluded — it lands with the
pending status-model redesign.

### A. Repairs (broken today)

1. CLI `features` (no `-p`) and `unregister`'s warning call `GET /api/features`,
   which does not exist → add a global `GET /api/features?project=` endpoint
   (mirrors `/api/queue` / `/api/review` shape) and keep the CLI calling it.
2. `projects` list command exists in `cli/commands/projects.py` but is not
   wired into the parser → add `borealis projects`.
3. CLI connect-error hint says `borealis start`, which doesn't exist → add
   `start`/`stop` lifecycle commands (systemd, mirroring Aurora's
   `cli/commands/lifecycle.py`).
4. `delete_task` commits via raw `repo.index` and never pushes → use
   `commit_and_push_board` with deleted paths (extend writer to stage
   removals).
5. README: remove `borealis logs`, fix `-p` → `--project`, add new commands
   once done.

### B. API gaps

6. `base_branch` on projects: add to `ProjectModel`, `projects.yaml`
   (default `main`), register body, and project responses. Aurora's review
   flow already expects it.
7. Archived feature visibility: `GET /api/projects/{project}/features` gains
   `?include=archived`; loader gains an archived-dir parse (read-only, not
   part of supervisor board state — loaded on demand).
8. Feature delete: `DELETE /api/projects/{project}/features/{feature}` for
   discarding a feature directory entirely (mistake cleanup; distinct from
   close/archive). Requires no tasks beyond draft, else 409.

### C. CLI write commands

All mutating commands print the API response message; destructive ones
(`task delete`, `feature delete`) take `-y/--yes` with a confirmation prompt.

```
borealis feature create  <project> <title> [--description] [--depends-on ...]
borealis feature show    <project> <feature>
borealis feature status  <project> <feature> <status>
borealis feature delete  <project> <feature> [-y]
borealis feature requeue <project> <feature>
borealis task create     <project> <feature> <title> --pipeline <name> [--body|--body-file] [--depends-on ...]
borealis task show       <project> <feature> <task_id>     (includes result content)
borealis task list       <project> <feature>
borealis task status     <project> <feature> <task_id> <status>
borealis task delete     <project> <feature> <task_id> [-y]
```

Implemented as two new command modules with `feature`/`task` argparse
sub-subparsers.

## Files to Modify

- `borealis/borealis/service/main.py` — global `GET /api/features`
- `borealis/borealis/service/models.py` — `base_branch` on `ProjectModel`
- `borealis/borealis/service/board/parser.py` / `writer.py` — `base_branch`
  in projects.yaml; stage deletions helper; archived feature loading
- `borealis/borealis/service/board/loader.py` — archived dir loader (on-demand)
- `borealis/borealis/service/api/features.py` — `include=archived` param,
  `DELETE` feature endpoint
- `borealis/borealis/service/api/tasks.py` — `delete_task` push fix
- `borealis/borealis/cli/main.py` — wire `projects`, `start`, `stop`,
  `feature`, `task` subcommands
- `borealis/borealis/cli/commands/lifecycle.py` — new (systemd start/stop)
- `borealis/borealis/cli/commands/feature.py` — new
- `borealis/borealis/cli/commands/task.py` — new
- `borealis/borealis/cli/commands/projects.py` — drop dead fallback handling
- `borealis/tests/test_api.py` — global features, base_branch, archived
  include, feature delete, task delete push
- `README.md` — CLI table update

## Todo

- [x] 1. Add `GET /api/features` (global, optional `project` filter)
- [x] 2. Wire `borealis projects`; add `start`/`stop` lifecycle commands
- [x] 3. Fix `delete_task` to commit-and-push via writer helper
- [x] 4. Add `base_branch` (model, yaml, register, responses, CLI display)
- [x] 5. Archived feature loading + `include=archived` query param
- [x] 6. `DELETE` feature endpoint (409 if non-draft tasks)
- [x] 7. CLI `feature` command group
- [x] 8. CLI `task` command group
- [x] 9. Tests for all of the above; README update; full suite + ruff

## Change History

- [2026-06-11] Plan created
- [2026-06-11] Implemented all items. `commit_board`/`commit_and_push_board`
  gained a `removed` param (staged deletions); `delete_task` and the new
  `DELETE /api/projects/{project}/features/{feature}` use it. `base_branch`
  added to `ProjectModel`, projects.yaml round-trip, register body, project
  responses, and `register --base-branch`. `load_archived_features` in
  loader.py backs `?include=archived` on the project features list. Global
  `GET /api/features?project=` added in main.py (fixes the CLI's dead calls).
  CLI: new `lifecycle.py` (start/stop), `feature.py` and `task.py` command
  groups wired with sub-subparsers (no-subcommand prints help); `features`
  gained `--archived`; `projects` command wired; register prints the
  server-derived name. README CLI table rewritten. 130/130 tests passing
  (6 new API tests, 1 parser round-trip test, 2 existing delete tests
  updated), ruff clean.

## Dependencies

- Independent of 015 (different files, except trivial overlap in `main.py`).
  017/018 (runtime work) do not depend on this plan.
