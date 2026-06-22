# 031 — Unified North CLI

## Context

Aurora and Borealis each ship their own CLI (`aurora`, `borealis`), built
independently (plans 007, 028). A review of both surfaced several
inconsistencies and gaps:

- **Inconsistent entity addressing**: `aurora approve/rollback/reject` take a
  combined `<project>/<feature>` string, while every `borealis feature`/`task`
  command takes `<project> <feature>` as two positionals.
- **Feature lifecycle is split across two binaries**: review verbs
  (`approve`/`rollback`/`reject`) live in `aurora`, while everything else about
  a feature (`create`/`show`/`status`/`delete`/`requeue`/`promote`) lives in
  `borealis feature`. Not discoverable — there's no `borealis feature approve`.
- **Duplicated code**: `_confirm()` is copy-pasted 4×, `lifecycle.py`
  (systemctl/linger wrapper) is byte-for-byte duplicated except for a unit
  name constant.
- **Missing functionality**: no `conversation status` (endpoint exists), no
  `projects show` (endpoint exists), no `task list --status` filter, no
  aggregate "show me both services" status view.

Rather than patch both CLIs in place — which would perpetuate the
two-binaries-drift problem as Aurora and Borealis continue to evolve
independently — we're creating a single new `north` CLI package that
**replaces both entirely**. One binary, one command tree, one test suite, one
place to keep `_confirm`/`lifecycle` shared logic. No back-compat shims; old
CLIs are deleted outright.

## Agreed command tree

```
north
├── status                       # both: aurora block + borealis block (partial-failure tolerant)
│   ├── aurora                   # aurora runner status only
│   └── borealis                 # borealis board status only
├── logs [--project]             # aurora SSE stream (was `aurora logs`)
├── pause / resume                # aurora runner control (was `aurora pause/resume`)
│
├── service
│   ├── aurora   <start|stop|restart|enable|disable|status>
│   ├── borealis <start|stop|restart|enable|disable|status>
│   └── status                   # NEW: aggregate status of both systemd units
│
├── projects
│   ├── list                      (was `borealis projects`)
│   ├── show <project>            # NEW: GET /api/projects/{project}
│   ├── register <ssh_url> [--name] [--base-branch] [--auto-merge]
│   ├── update <project> [--base-branch] [--auto-merge/--no-auto-merge]
│   └── unregister <project> [-y]
│
├── feature
│   ├── create / show / status / delete / requeue / promote   (Borealis-backed, unchanged)
│   ├── approve / rollback / reject                             (Aurora-backed, was `aurora approve/rollback/reject`,
│   │                                                              now two positionals <project> <feature>)
│   └── list [--project] [--archived] [--review]   # --review replaces standalone `borealis review`
│
├── task
│   └── create / show / list [--status] / status / delete / promote / split
│        (NEW: --status filter on list, client-side)
│
├── conversation
│   └── create / list / show / status <project> <conv_id> <status>   (NEW: status)
│
└── comment
    └── add / list
```

**`projects update`** (folded in 2026-06-16, was a scoped-out follow-up): edit a
project's `base_branch`/`auto_merge` after registration. Requires a new
`PATCH /api/projects/{project}` route in `borealis/borealis/service/main.py`
(request model `ProjectUpdateBody` with optional `base_branch`/`auto_merge`,
404 on unknown project, 400 if no fields given), persisting to `projects.yaml`
the same way `register`/`unregister` do (parse → mutate → write → commit/push →
update in-memory state), plus the `north projects update` command. `ssh_url` and
`name` are immutable (identity / clone source) and are not editable here.

## Package layout

New workspace member `north/`:

```
north/
  pyproject.toml
  north/
    __init__.py
    cli/
      __init__.py
      __main__.py
      main.py                 # top-level parser + dispatch
      context.py              # NorthContext (lazy aurora/borealis clients)
      prompts.py              # shared confirm()
      clients/
        __init__.py
        errors.py             # shared CLIError
        aurora.py             # AuroraClient (ported from aurora/aurora/cli/client.py)
        borealis.py           # BorealisClient (ported from borealis/borealis/cli/client.py)
      commands/
        __init__.py
        status.py             # north status / status aurora / status borealis
        observe.py            # north logs
        control.py            # north pause / resume
        lifecycle.py           # unit-parametrized systemctl/linger wrapper
        service.py            # north service status (aggregate)
        projects.py           # list/show/register/unregister
        feature.py            # create/show/status/delete/requeue/promote/list
        review.py             # approve/rollback/reject
        task.py               # create/show/list/status/delete/promote/split
        conversation.py       # create/list/show/status
        comment.py            # add/list
  tests/
    __init__.py
    conftest.py               # MockTransport helpers + NorthContext.for_testing
    test_cli_status.py
    test_cli_observe.py
    test_cli_control.py
    test_cli_lifecycle.py
    test_cli_service.py
    test_cli_projects.py
    test_cli_feature.py
    test_cli_review.py
    test_cli_task.py
    test_cli_conversation.py
    test_cli_comment.py
```

### `north/pyproject.toml`

```toml
[project]
name = "north"
version = "0.1.0"
requires-python = ">=3.12"
dependencies = [
    "aurora",
    "borealis",
    "httpx",
]

[tool.uv.sources]
aurora = { workspace = true }
borealis = { workspace = true }

[project.optional-dependencies]
dev = [
    "ruff",
    "mypy",
    "pytest",
    "pytest-asyncio",
]

[project.scripts]
north = "north.cli.main:main"

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"

[tool.hatch.build.targets.wheel]
packages = ["north"]
```

`north`'s `clients/aurora.py` / `clients/borealis.py` import
`from aurora.service.config import settings` / `from borealis.service.config
import settings` for port resolution — hence the workspace path deps. No
`[tool.ruff]`/`[tool.mypy]` overrides needed; root `pyproject.toml` config
applies repo-wide (matches how `aurora`/`borealis` pyproject.toml work today).

Root `pyproject.toml`:
```toml
[tool.uv.workspace]
members = ["aurora", "borealis", "north"]
```

## Key design points

### `NorthContext` (`context.py`)
Lazily constructs `AuroraClient`/`BorealisClient` on first property access
(`ctx.aurora`, `ctx.borealis`), so single-service commands don't depend on the
other service being reachable. Context manager closes whichever clients were
actually constructed. Every command handler signature becomes
`(args: argparse.Namespace, ctx: NorthContext) -> int` — replacing the old
`needs_client`-flag dance in Aurora's `main.py` (no longer needed; `NorthContext`
construction is always free).

Add a `for_testing(*, aurora=None, borealis=None)` classmethod for test
ergonomics (sets `_aurora`/`_borealis` directly).

### Shared `CLIError` (`clients/errors.py`)
One `CLIError` class, imported by both `clients/aurora.py` and
`clients/borealis.py`, so `main.py`'s single `except CLIError` clause and
`north status`'s per-block try/except both work regardless of which service
raised. Connect-error hints update to `north service aurora start` /
`north service borealis start`.

### Shared `confirm()` (`prompts.py`)
Single implementation, replacing the 4 duplicated `_confirm()` copies in
`feature.py`, `task.py`, `review.py`, `projects.py`.

### `lifecycle.py` — unit-parametrized
Today's `lifecycle.py` hardcodes `_UNIT` at module scope; duplicated verbatim
between Aurora and Borealis otherwise. New version takes the unit name via
`args.unit` (set with `set_defaults(unit="aurora"|"borealis")` at parser-build
time for each `service <unit> <action>` subparser) — one handler signature
`(args, ctx) -> int` throughout, matching every other command. All helper
functions (`_action`, `_show_properties`, `_run_systemctl`, `_linger_enabled`,
`_boot_line`, `_PAST_TENSE`) take `unit: str` as an explicit parameter instead
of reading a module global.

Refactor the status-printing logic into `_status_lines(unit: str) -> list[str]`
so both `lifecycle.status` (single unit) and `service.aggregate_status` (calls
it twice, for `north service status`) can reuse it. `_show_properties` already
degrades gracefully for a stopped/missing unit (returns `{}`), so aggregate
status doesn't need special-case error handling beyond what exists today.

### `north status` (combined)
Each block (`aurora`, `borealis`) is fetched independently; a `CLIError` in
one block prints `"  error: ..."` under that block's heading without aborting
the other. Overall exit code is non-zero if either block failed.
`status aurora` / `status borealis` reuse the same per-block printer for the
single-service case.

### `feature list`
Consolidates `borealis features` and `borealis review`:
```python
def list_(args, ctx) -> int:
    client = ctx.borealis
    if args.review and args.archived:
        raise CLIError("--review and --archived are mutually exclusive")
    if args.review:
        items = client.get("/api/review", params={"project": args.project} if args.project else None)
        _print_features(items, "no features awaiting review")
        return 0
    if args.archived and not args.project:
        raise CLIError("--archived requires --project")
    if args.project:
        params = {"include": "archived"} if args.archived else None
        items = client.get(f"/api/projects/{args.project}/features", params=params)
    else:
        items = client.get("/api/features")
    _print_features(items, "no features")
    return 0
```
`_print_features` moves from `borealis/cli/commands/observe.py` into
`north/cli/commands/feature.py`.

### `feature approve/rollback/reject`
Ported from `aurora/cli/commands/review.py`, dropping `_split_target` —
argparse now declares `project` and `feature` as two separate positionals,
matching every other `feature`/`task` subcommand.

### `task list --status`
`borealis/borealis/service/api/tasks.py:list_tasks(project, feature, ctx)` has
no `status` query param (confirmed) — implement as a client-side filter:
fetch the full list, then `[t for t in items if t.get("status") == args.status]`
when `--status` is given.

### `conversation status` (new)
```python
def status(args, ctx) -> int:
    data = ctx.borealis.patch(
        f"/api/projects/{args.project}/conversations/{args.conversation_id}/status",
        body={"status": args.status},
    )
    print(f"{args.project}/{args.conversation_id} → {data.get('status', args.status)}")
    return 0
```
Three positionals: `<project> <conversation_id> <status>`, mirroring
`feature status` / `task status`.

### `projects show` (new)
```python
def show(args, ctx) -> int:
    data = ctx.borealis.get(f"/api/projects/{args.project}")
    for key in ("name", "ssh_url", "base_branch", "auto_merge"):
        print(f"{key + ':':13} {data.get(key)}")
    return 0
```

### Installation / `north` on PATH
Keep the standard `[project.scripts] north = "north.cli.main:main"` entry point
so North is conventionally installable. CLI exposure is split by audience —
`scripts/install.sh` does **not** install or symlink the CLI:

**Production (operators).** The systemd services run via `uvicorn` and don't
need the CLI, so `install.sh` provisions services only (no CLI install, no
symlink). Operators who want the `north` command follow a README note and
install it the conventional isolated way:
```bash
uv tool install north      # or: pipx install north
```

**Development.** A new `scripts/install-dev.sh` symlinks the editable console
script from the workspace `.venv` (already built by `install.sh`'s
`uv sync --all-extras`) into `~/.local/bin` (the repo's standard tool dir — the
systemd units invoke `%h/.local/bin/uv`):
```bash
mkdir -p "$HOME/.local/bin"
ln -sf "$NORTH_HOME/.venv/bin/north" "$HOME/.local/bin/north"

case ":$PATH:" in
  *":$HOME/.local/bin:"*) ;;
  *) echo "WARNING: add \$HOME/.local/bin to your PATH to use 'north'";;
esac
```
The editable workspace install means source edits to `north`/`aurora`/`borealis`
are picked up with no reinstall, and the symlink adds zero call-time overhead
(it's the real console script, not a `uv run` wrapper) — exactly what you want
while iterating on the CLI.

## Argparse structure notes

- Top level: `sub = parser.add_subparsers(dest="command")`.
- Group parsers with subcommands (`feature`, `task`, `conversation`, `comment`,
  `projects`, `service`, `service aurora`, `service borealis`) each need a
  `set_defaults(func=...)` fallback that prints that parser's help and returns
  1 when no subcommand is given — replicate the existing `_service_help`
  pattern via one shared helper `_group_help(parser) -> Callable`, defined once
  in `main.py` and reused for every group.
- `service <unit> <action>`: 3 levels deep. `set_defaults(unit="aurora")` on
  the `service aurora` parser propagates to all of its child action parsers'
  namespaces (argparse merges `set_defaults` along the matched chain). Smoke
  test this during implementation with
  `parser.parse_args(["service", "aurora", "start"])` → confirm
  `args.unit == "aurora"`.
- `--project` filter: keep the existing small `_project_opt(p)` closure
  pattern, used by `logs` and `feature list`.

## Command mapping (old → new)

| Old | New |
|---|---|
| `aurora status` | `north status aurora` |
| `borealis status` | `north status borealis` |
| *(new)* | `north status` |
| `aurora logs [--project]` | `north logs [--project]` |
| `aurora pause` / `resume` | `north pause` / `resume` |
| `aurora service ...` | `north service aurora ...` |
| `borealis service ...` | `north service borealis ...` |
| *(new)* | `north service status` |
| `borealis projects` | `north projects list` |
| *(new)* | `north projects show <project>` |
| `borealis register ...` | `north projects register ...` |
| *(new)* | `north projects update <project> [--base-branch] [--auto-merge/--no-auto-merge]` |
| `borealis unregister ...` | `north projects unregister ...` |
| `borealis feature {create,show,status,delete,requeue,promote}` | unchanged under `north feature` |
| `aurora {approve,rollback,reject} <p>/<f>` | `north feature {approve,rollback,reject} <p> <f>` |
| `borealis features [--project] [--archived]` | `north feature list [--project] [--archived]` |
| `borealis review [--project]` | `north feature list [--project] --review` |
| `borealis task {create,show,list,status,delete,promote,split}` | unchanged under `north task` (+ `list --status`) |
| `borealis conversation {create,list,show}` | unchanged under `north conversation` |
| *(new)* | `north conversation status <p> <conv_id> <status>` |
| `borealis comment {add,list}` | unchanged under `north comment` |

## Testing

Mirror existing conventions (`httpx.MockTransport` for HTTP commands,
`monkeypatch.setattr(lifecycle.subprocess, "run", fake_run)` for lifecycle,
`_build_parser().parse_args(...)` + `MagicMock()` for dispatch tests).
`test_cli_lifecycle.py` parametrizes over `unit in ("aurora", "borealis")`.
`test_cli_status.py` exercises the combined command's partial-failure path
(one client errors, the other succeeds) via `NorthContext.for_testing`.

Update README's test command to:
```
uv run --with pytest,pytest-asyncio,httpx python -m pytest aurora/tests/ borealis/tests/ north/tests/ --ignore=aurora/tests/integration
```

## Todo

- [x] 1. Scaffold `north/pyproject.toml`; add `north` to root `pyproject.toml` workspace members; verify `uv sync` succeeds. [2026-06-15]
- [x] 2. Create `north/north/cli/clients/errors.py` (shared `CLIError`), `clients/aurora.py`, `clients/borealis.py` (ported from existing `cli/client.py` files). [2026-06-15]
- [x] 3. Create `north/north/cli/context.py` (`NorthContext` + `for_testing`) and `north/north/cli/prompts.py` (`confirm()`). [2026-06-15]
- [x] 4. Port `commands/lifecycle.py` as unit-parametrized (reads `args.unit`, `_status_lines` helper); port `commands/service.py` (`aggregate_status`). [2026-06-15]
- [x] 5. Port `commands/observe.py` (logs) and `commands/control.py` (pause/resume) — client → `ctx.aurora`. [2026-06-15]
- [x] 6. Port `commands/status.py` (combined / aurora / borealis, partial-failure tolerant). [2026-06-15]
- [x] 7. Port `commands/projects.py` (list/show/register/unregister, shared `confirm`). [2026-06-15]
- [x] 8. Port `commands/feature.py` (create/show/status/delete/requeue/promote/list incl. `--review`/`--archived`, shared `confirm`). [2026-06-15]
- [x] 9. Port `commands/review.py` (approve/rollback/reject) — two positionals, shared `confirm`. [2026-06-15]
- [x] 10. Port `commands/task.py` (create/show/list incl. `--status`/status/delete/promote/split, shared `confirm`). [2026-06-15]
- [x] 11. Port `commands/conversation.py` (create/list/show + new `status`). [2026-06-15]
- [x] 12. Port `commands/comment.py` (add/list). [2026-06-15]
- [x] 13. Write `cli/main.py` (`_build_parser`, `_group_help`, `main`, `_entrypoint`) and `cli/__main__.py`; smoke-test 3-level `service aurora start` nesting. [2026-06-15]
- [x] 14. Write `north/tests/conftest.py` + `test_cli_*.py` for all command modules (+ `test_cli_main.py` dispatch tests). 80 north tests pass. [2026-06-16]
- [x] 15. Delete `aurora/aurora/cli/`, `aurora/tests/test_cli_*.py`; remove `[project.scripts]` entry from `aurora/pyproject.toml`. [2026-06-16]
- [x] 16. Delete `borealis/borealis/cli/`, `borealis/tests/test_cli_*.py`; remove `[project.scripts]` entry from `borealis/pyproject.toml`. [2026-06-16]
- [x] 17. Create `scripts/install-dev.sh`: symlink `<repo>/.venv/bin/north` → `~/.local/bin/north` with the `~/.local/bin` PATH check/warning. (`scripts/install.sh` unchanged — no CLI install/symlink.) [2026-06-16]
- [x] 18. Update `README.md`: replaced the two CLI sections with one "North CLI" section; documented `uv tool install north` for operators and `scripts/install-dev.sh` for the editable dev symlink; updated Development commands and repository layout diagram. [2026-06-16]
- [x] 19. Add supersession notes to `docs/plans/007_cli.md` and `docs/plans/028_cli-service-control.md` pointing at this plan. [2026-06-16]
- [x] 20. Ran `uv run ruff check .` (clean), `uv run mypy north/north` (clean), and the full test suite: 342 passed, 1 pre-existing unrelated failure (`test_session_runner.py::test_no_provider_falls_back_to_local`, ollama provider fallback — not touched by this work). Repo-wide mypy still reports pre-existing errors in aurora/borealis *service* code (out of scope). [2026-06-16]
- [x] 21. `projects update` — add `PATCH /api/projects/{project}` endpoint (Borealis) + `north projects update` command, with tests on both sides. [2026-06-16]

### Parallelization notes
Items 4–12 (command module ports) are independent once 1–3 land, and can run
as parallel sub-agent tasks. Item 13 (`main.py`) depends on all of 4–12. Item
14 (tests) can proceed per-module in parallel with other modules, though
dispatch-level tests depend on 13. Items 15–16 (deletions) run only after
1–14 are verified working. Items 17–18 (docs) are independent and can run in
parallel with 4–14. Item 17 (`install-dev.sh`) is independent and can run
in parallel with 4–14.

## Change history
- [2026-06-15] Plan created.
- [2026-06-15] Added "Installation / `north` on PATH" design point: retain the
  `[project.scripts]` entry point for conventional installs (`pipx`/`uv tool`);
  `scripts/install.sh` symlinks the workspace `.venv` console script into
  `~/.local/bin` for our deployment. Added todo 17 (install.sh) and renumbered
  18–21.
- [2026-06-15] Split CLI exposure by audience: `install.sh` stays CLI-free
  (services only); operators use `uv tool install north` (documented in README);
  new `scripts/install-dev.sh` owns the editable `~/.local/bin` symlink for
  development. Reworked todo 17 → `install-dev.sh` and updated README todo 18.
- [2026-06-16] Implemented the full plan (todos 1–20). Foundation + 11 command
  modules ported; `main.py` wires the command tree (3-level `service <unit>
  <action>` nesting verified, `unit` propagation working). 80 north tests added
  (incl. dispatch tests in `test_cli_main.py`). Old `aurora`/`borealis` CLI
  packages, their `test_cli_*` tests, and `[project.scripts]` entries deleted;
  `north` is the only console script. `scripts/install-dev.sh` added; README and
  supersession notes (007/028) updated. Final: ruff clean, north mypy clean,
  342 passed / 1 pre-existing unrelated failure. Todo 21 (`projects update`)
  remains a deliberate follow-up.
- [2026-06-16] Folded in the former follow-up (todo 21) and implemented it:
  added `PATCH /api/projects/{project}` to Borealis (`ProjectUpdateBody`,
  base_branch/auto_merge optional, 404 unknown / 400 no-fields, persists to
  `projects.yaml` + updates in-memory state) and the `north projects update`
  command (`--base-branch`, mutually-exclusive `--auto-merge`/`--no-auto-merge`).
  8 new tests (5 endpoint via `TestClient(main.app)` without lifespan, 3 CLI).
  `ssh_url`/`name` kept immutable. Full suite: 350 passed / 1 pre-existing
  unrelated failure; ruff + north mypy clean.
- [2026-06-16] Note: `north/tests/` intentionally has **no** `__init__.py` —
  making it a `tests` package collided with `aurora/tests` (also `tests`) during
  combined collection (`ModuleNotFoundError: No module named 'tests.…'`). Matches
  the borealis convention.
