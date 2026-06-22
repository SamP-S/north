# Plan: Fold Borealis + CLI into a single installable `north` package, purge Aurora

## Context

This repo (`SamP-S/north`) is the "board-keeping" half of a fork that split the original
North monorepo into two repositories. The other repo keeps **Aurora** (the agentic execution
engine). **This repo keeps only the board.**

Today the repo still carries the pre-fork shape:
- `borealis/` — the board service (FastAPI + git-backed board + MCP + orchestrator).
- `north/` — a CLI that was an umbrella interface over *both* aurora and borealis.
- A uv workspace (`pyproject.toml` members `["aurora", "borealis", "north"]`) — but `aurora/`
  is already gone, leaving a dangling member and dead `aurora` dependency.

**Goal / outcome:** collapse everything into **one** package called `north` that contains the
board (`north/service/`) and the CLI (`north/cli/`), with **all Aurora references removed** and
the package **installable as a local tool** (`uv tool install .` → `north` command). After this,
"north" is a single board-only project, not a monorepo.

### Decisions locked with the user
1. **Structure:** flatten to a single top-level package at repo root (one `north/` package +
   one root `pyproject.toml`, no uv workspace) — the layout that supports installing `north` as a tool.
2. **CLI Aurora commands:** *drop* aurora-only commands entirely (not stub them).
3. **`aurora/brief` author string:** rename the magic author to **`north/brief`** everywhere.
4. **Config rename:** `borealis_port` → `north_port` (`NORTH_PORT`, keep 8001); board path
   default `~/.north/borealis/board` → `~/.north/board`.
5. **Docs purge:** active docs only (README, CLAUDE.md, `docs/design/*`); leave `docs/plans/`
   and `docs/archive/` as historical record.

---

## Target layout (after flatten)

```
north/                     # repo root
  pyproject.toml           # single package "north": deps, console script, ruff/mypy/pytest
  north/                   # the import package
    __init__.py
    service/               # <- from borealis/borealis/service/
    cli/                   # <- from north/north/cli/
  tests/                   # merged borealis/tests + north/tests
  docs/  scripts/  systemd/
  README.md  CLAUDE.md  uv.lock
```
Console entry point: `[project.scripts] north = "north.cli.main:main"`.
Run the service with `uvicorn north.service.main:app` (no more `--package`).

---

## Files to modify (by area)

### A. Package restructure (git mv — preserve history)
- `borealis/borealis/service/**` → `north/service/**`
- `north/north/cli/**` → `north/cli/**` (the wrapper `north/north/` collapses into `north/`)
- `north/north/__init__.py` → `north/__init__.py` (keep/rewrite metadata, no aurora)
- `borealis/tests/**` + `north/tests/**` → `tests/**`
- Delete: `borealis/` dir, `borealis/pyproject.toml`, `north/pyproject.toml`, old nesting.

### B. Import + patch-path rewrite (mechanical, repo-wide in moved code)
- `from borealis.service.*` / `import borealis.service.*` → `north.service.*`
- CLI board client: `north/cli/clients/borealis.py` — `from borealis.service.config import settings`
  → `from north.service.config import settings`.
- Test mocks: `patch("borealis.service.*")` → `patch("north.service.*")`.

### C. Drop Aurora from the CLI (decision: drop)
Delete files:
- `north/cli/clients/aurora.py`
- `north/cli/commands/control.py`  (aurora pause/resume)
- `north/cli/commands/review.py`   (aurora approve/rollback/reject)
- Tests: `tests/test_cli_control.py`, `tests/test_cli_review.py`

Strip aurora from mixed files:
- `north/cli/commands/observe.py` — remove `logs()` (aurora SSE); keep `queue()` (board).
- `north/cli/commands/status.py` — remove `_aurora_block()`, the `aurora()` subcommand, and the
  aurora call inside combined `status()`; keep the borealis block.
- `north/cli/commands/service.py` — change `for unit in ("aurora", "borealis")` → just `"north"`.
- `north/cli/commands/lifecycle.py` — default unit becomes `north`; remove aurora unit wiring.
- `north/cli/main.py` — remove imports of `control`/`review`; remove subparsers for `status aurora`,
  `control pause/resume`, `service aurora`, `observe logs`, and `feature approve/rollback/reject`.
- `north/cli/context.py` — remove the `_aurora` attribute + lazy `aurora` property (board-only ctx).
- Trim aurora cases from: `tests/test_cli_observe.py`, `tests/test_cli_status.py`,
  `tests/test_cli_service.py`, `tests/test_cli_lifecycle.py`, `tests/test_cli_main.py`,
  `tests/conftest.py` (aurora fixtures/mocks).
- Update the board-client error hint `try \`north service borealis start\`` → `north service start`
  (single unit now).

### D. Board branding + config rename
- `north/service/config.py` — `borealis_port` → `north_port` (env `NORTH_PORT`, default 8001);
  `board_path` default → `Path("~/.north/board").expanduser()`.
- `north/service/main.py` — `FastAPI(title="Borealis")` → `title="North"`.
- `north/service/mcp.py` — MCP server name `f"borealis-{grant}"` → `f"north-{grant}"`; instructions
  string `"Borealis board access…"` → `"North board access…"`; module docstring.
- `north/service/logsetup.py` — docstrings "Borealis" → "North".
- `north/service/api/comments.py` — `BRIEF_AUTHOR = "aurora/brief"` → `"north/brief"` (decision #3).
- Tests: `tests/test_gate_events.py` + `tests/test_comments.py` — update `aurora/brief` → `north/brief`
  and `aurora/implement` → `north/implement`; `tests/test_api.py` docstring "Borealis" → "North".

### E. Single root `pyproject.toml` (merge)
Combine into one `[project] name = "north"`:
- Dependencies = board deps (`fastapi`, `uvicorn[standard]`, `python-frontmatter`, `gitpython`,
  `pydantic`, `pydantic-settings`, `httpx`, `python-dotenv`, `sse-starlette`, `pyyaml`, `mcp`).
  Drop `aurora` and `borealis` workspace deps/sources entirely.
- `[project.scripts] north = "north.cli.main:main"`.
- `[build-system]` hatchling; `[tool.hatch.build.targets.wheel] packages = ["north"]`.
- Keep dev extras (`ruff`, `mypy`, `pytest`, `pytest-asyncio`).
- Fold in root tooling: `[tool.ruff]`, `[tool.ruff.lint]`, `[tool.mypy]`, `[tool.pytest.ini_options]`.
- **Remove** `[tool.uv.workspace]` (no more multi-package workspace).
- Regenerate `uv.lock` via `uv sync`.

### F. systemd
- Delete `systemd/aurora.service`.
- Rename `systemd/borealis.service` → `systemd/north.service`: `Description=North board state service`;
  `ExecStart=… uvicorn north.service.main:app …` (drop `--package`); `Environment=NORTH_PORT=8001`,
  use `${NORTH_PORT}`.
- `systemd/north-notify-failure@.service` — unchanged.
- `systemd/opencode.service` — out of scope for the board (external agent runtime); leave the file
  but no longer referenced by install. (Flag to user — see Open item.)

### G. scripts
- `scripts/install.sh` — remove `AURORA_HOME`/`AURORA_PORT`/`AURORA_DIR`, the `~/.north/aurora`
  worktree/repos setup, `aurora.service` install, the `--package aurora` OAuth test, and aurora
  summary lines; install/enable only `north.service`; board path → `~/.north/board`; prefer
  `uv tool install .` to expose the `north` CLI (installable-as-tool goal).
- `scripts/install-dev.sh` — update the `north/aurora/borealis` comment to just `north`.
- `scripts/cockpit.sh`, `scripts/notify-failure.sh` — no changes.

### H. Active docs (decision: active only)
- `CLAUDE.md` — rewrite the project description (currently "two background services (Aurora and
  Borealis)…") to a board-only North description; update the monorepo framing.
- `README.md` — remove Aurora sections (session execution, pipelines/gates, decomposition, review
  briefs, refine rule, voice), simplify the services table to North only, fix install steps
  (no auth test / opencode pin / worktrees), update CLI command list to surviving commands.
- `docs/design/01_v2-architecture.md` — drop Aurora + AgentRuntime-adapter + migration sections;
  reframe as "North is a git-backed board service; agent runtime is external."
- `docs/design/99_planned-features.md` — delete agent-execution-only features (LangGraph,
  reconciliation, context agents, memory, staging workflow, aurora metrics/usage caps); keep
  board-level features.
- Leave `docs/plans/**` and `docs/archive/**` untouched.

---

## Ordered todo

- [ ] 1. Copy this plan to `docs/plans/036_fork-fold-north.md` (project convention: numbered plan in docs/plans).
- [ ] 2. **Restructure (A):** git mv board + CLI into top-level `north/` package; merge tests into `tests/`; delete `borealis/` + old wrapper pyprojects.
- [ ] 3. **Imports (B):** rewrite `borealis.service.*` → `north.service.*` in code + tests + patch strings.
- [ ] 4. **Root pyproject (E):** write single `north` pyproject; remove workspace; `uv sync`.
- [ ] 5. **Drop aurora CLI (C):** delete control/review/aurora-client + tests; strip aurora from observe/status/service/lifecycle/main/context; trim aurora test cases.
- [ ] 6. **Board branding + config (D):** title, MCP names, logsetup, `north_port`/board path, `BRIEF_AUTHOR="north/brief"`, test updates.
- [ ] 7. **systemd (F):** delete aurora.service; rename borealis→north.service + content.
- [ ] 8. **scripts (G):** rewrite install.sh; fix install-dev.sh comment.
- [ ] 9. **Active docs (H):** CLAUDE.md, README.md, docs/design/01 + 99.
- [ ] 10. **Verify** (see below); fix lint/type/test fallout.

## Change history
- [2026-06-22] Plan drafted.
- [2026-06-23] Implemented end-to-end. Flattened to a single `north/` package
  (`north/service/` + `north/cli/`, tests in `tests/`, single root pyproject, no uv workspace;
  `borealis/` deleted). Rewrote `borealis.service.*` imports → `north.service.*`. Dropped aurora
  CLI (deleted `clients/aurora.py`, `commands/control.py`/`review.py`/`service.py`, the `logs`
  command and `status aurora`/combined); renamed board client `BorealisClient`→`BoardClient`
  (`clients/board.py`), `ctx.borealis`→`ctx.board`, `BOREALIS_PORT`→`NORTH_PORT`; collapsed
  `service` to the single `north` unit. Branding/config: title "North", MCP `north-<grant>`,
  `north_port`, board path `~/.north/board`, `BRIEF_AUTHOR="north/brief"`. systemd: removed
  `aurora.service`, renamed `borealis.service`→`north.service`; rewrote `install.sh` (board-only +
  `uv tool install`). Docs: README, CLAUDE.md, `docs/design/01`+`99` rewritten board-only.
  Verified: ruff clean, 218 tests pass, grep gate clean, `north` installs as a tool and reaches
  the service.
  NOTE: `uv run mypy north` reports 61 strict errors — all pre-existing board-service tech debt
  (bare `dict` annotations, an MCP mount signature), none in `north/cli`, none introduced here.
  Left for a follow-up cleanup (out of scope for the fork).
- [2026-06-23] Removed `systemd/opencode.service` and `systemd/north-notify-failure@.service`
  (and the orphaned `scripts/notify-failure.sh`); dropped the `OnFailure=` line from
  `north.service`, the notify-template install from `install.sh`, and the layer-b health note +
  repo-layout listing from README. North now installs a single systemd unit (`north.service`).
- [2026-06-23] Removed Aurora operator tooling: `scripts/cockpit.sh` (tmux + Claude-Code cockpit
  workspace) and `docs/examples/` (pipeline examples). The MCP `cockpit` grant in `north/service`
  stays — it's a board grant, not the script.
- [2026-06-23] Rebuilt `docs/design/` as a board-only North spec, carrying the relevant v1
  architecture forward (de-aurora'd, borealis→north): `00_index`, `01_overview`, `02_architecture`,
  `03_repository-layout`, `04_board-data-model`, `05_git-conventions`, `06_backend-api` (grounded in
  actual routes + MCP), `07_cli`, `08_notifications`, `09_configuration`, `10_testing`; kept
  `99_planned-features`; removed the thin `01_v2-architecture.md`. `docs/archive/design/v1/` (still
  contains aurora) retained as the historical source — flag for removal if no longer wanted.

---

## Verification

Run from repo root after changes:
1. `uv sync` — resolves with the single merged pyproject (no aurora/borealis members), no errors.
2. `uv run ruff check .` — clean.
3. `uv run mypy north` — clean (strict).
4. `uv run pytest` — all surviving board + CLI tests pass; no `borealis.`/`aurora` import or patch errors.
5. Service boots: `uv run uvicorn north.service.main:app --port 8001` → `GET /api/status` 200;
   OpenAPI title shows "North".
6. CLI works: `uv run north --help` lists only surviving commands (no control/review/aurora/logs);
   `uv run north status` hits the board.
7. Tool install: `uv tool install .` then `north --help` resolves on PATH (installable-as-tool goal).
8. Grep gate: `grep -rni aurora north tests scripts systemd README.md CLAUDE.md docs/design pyproject.toml`
   returns nothing (docs/plans + docs/archive intentionally excluded).

---

## Open item to confirm during implementation
- `systemd/opencode.service` (external opencode agent runtime) isn't part of the board. Plan keeps
  the file but stops installing/enabling it. If you'd rather delete it too, say so.
