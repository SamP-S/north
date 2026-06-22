# 001 — Board Core

## Summary

Lay the foundation: project structure, board repo parsers, prefixed-commit writer, and a FastAPI skeleton with stub endpoints. Goal: `aurora status` and `aurora queue` can connect to a running service and see board state. No agents, no execution.

## Files to Create / Modify

```
pyproject.toml
service/
  __init__.py
  main.py                      # FastAPI app + uvicorn entry point
  config.py                    # settings loaded from .env
  models.py                    # TaskState, FeatureState, EpicState dataclasses
  board/
    __init__.py
    parser.py                  # frontmatter parsers: task, feature, epic, projects.yaml
    writer.py                  # prefixed commits + push to board repo
    loader.py                  # load full board state from board repo into memory
tests/
  __init__.py
  test_parser.py
  test_writer.py
```

## Todo

- [x] 1. Create `pyproject.toml` — project metadata, deps (`fastapi`, `uvicorn[standard]`, `python-frontmatter`, `gitpython`, `pydantic`, `httpx`, `python-dotenv`), dev deps (`ruff`, `mypy`, `pytest`, `pytest-asyncio`)
- [x] 2. `service/config.py` — load all `.env` variables from §11.1; expose as a `Settings` dataclass; validate required fields on import
- [x] 3. `service/models.py` — `TaskState`, `FeatureState`, `EpicState` dataclasses with all fields from §7.2; `TaskStatus`, `FeatureStatus` enums
- [x] 4. `service/board/parser.py` — parse task frontmatter (all fields from §7.2, normalise `depends_on` to zero-padded strings); parse feature frontmatter; parse epic frontmatter; parse `projects.yaml`
- [x] 5. `service/board/writer.py` — `commit_board(repo, message, paths)` with prefixed commit message; `push_board(repo)` with rebase-retry conflict handling (§8.5)
- [x] 6. `service/board/loader.py` — walk board repo `projects/{project}/board/` and build in-memory `dict[str, FeatureState]` + task lists; expose `load_board_state(board_path) -> BoardState`
- [x] 7. `service/main.py` — FastAPI app; `GET /api/health`; `GET /api/status` (stub: runner state idle, no active task); `GET /api/queue` (read from in-memory board state); `GET /api/projects`; `GET /api/features`; `GET /api/events` (stub SSE); `POST /api/control` (stub)
- [x] 8. Unit tests — parser round-trips for task/feature/epic/projects.yaml; `depends_on` normalisation (string and int inputs); missing optional fields handled gracefully
- [x] 9. Run `uv run ruff check .` and `uv run mypy service/` — fix all errors

## Change History

- [2026-06-07] All items complete. 9/9 tests pass, ruff clean, mypy clean. Notes: used `StrEnum` (UP042); `board_repo_ssh_url` defaults to `""` (validated at startup in plan 008); `meta` cast to `dict[str, Any]` to satisfy mypy on frontmatter; `.env` created with test board `git@github.com:SamP-S/aurora-board-test.git`; `docs/` excluded from ruff (pre-existing example files).
