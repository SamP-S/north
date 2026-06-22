# 008 — Hardening

## Summary

Production-readiness: service startup validation, crash/restart recovery, install script, systemd unit files, OAuth smoke test, board schema migration framework, and the full integration + smoke test suite. Depends on all prior stages.

## Files to Create / Modify

```
service/
  startup.py                   # startup validation + in_progress reset
scripts/
  install.sh                   # idempotent install script
systemd/
  aurora.service               # user systemd unit
  ollama.service               # user systemd unit (if not system-managed)
tests/
  integration/
    test_full_task_run.py      # board → queue → execute → board update
    test_feature_lifecycle.py  # open → in_progress → review → merged
    test_pause_resume.py       k# pause at safe boundaries
    test_rate_limit.py         # RateLimitEvent → queue pause
    test_soft_cap.py           # spend >= soft cap → queue pause
  smoke/
    SMOKE_TEST.md              # manual smoke test checklist
```

## Todo

- [x] 1. `service/startup.py` — `run_startup_validation(aurora_path, board_path)`: check `board/` exists and is a valid Git repo (fail with clear log if not); check `board/projects.yaml` exists; if missing, create with empty registry and commit `[board:project]`; after checks pass, reset any `in_progress` tasks to `queued` (§5.7); log each reset task
- [x] 2. `service/main.py` — call `run_startup_validation()` before launching supervisor loop; fail fast if board repo check fails (service exits with non-zero, systemd restart policy handles it)
- [x] 3. `scripts/install.sh` — idempotent install steps from §12.3: pin Python 3.12; `uv sync`; install Claude Code CLI; prompt for `claude auth login` (or `claude setup-token` for headless); create `$AURORA_HOME/{repos,worktrees,data}/`; clone board repo to `aurora/board/` (abort with clear error if remote not found); `loginctl enable-linger`; render + `systemctl --user enable --now` all units; verify Ollama reachable + required models present; run OAuth smoke test (step 8); print access details
- [x] 4. `scripts/install.sh` — OAuth smoke test: `python -c "import asyncio; from claude_agent_sdk import query; ..."` minimal single-turn call; assert returns without `authentication_failed`; fail install with clear message if it fails
- [x] 5. `systemd/aurora.service` — user unit: `Type=simple`; `ExecStart=uv run uvicorn service.main:app --host 127.0.0.1 --port ${AURORA_PORT:-8000}`; `EnvironmentFile=%h/.aurora/.env` (or aurora repo `.env`); `Restart=on-failure`; `RestartSec=5`
- [x] 6. `systemd/ollama.service` — user unit for Ollama if not managed as a system service: `ExecStart=ollama serve`; `Restart=on-failure`
- [x] 7. Migration framework — `scripts/migrate.py`: base runner that discovers and applies numbered migration scripts in `scripts/migrations/`; migrations are plain Python scripts with an `up()` function; record applied migrations in `$AURORA_HOME/data/migrations.json`; run idempotently
- [x] 8. Integration tests — full task run, feature lifecycle, pause/resume, rate limit, soft-cap
- [x] 9. `tests/smoke/SMOKE_TEST.md` — manual checklist per §13.3
- [x] 10. Run full test suite: 117 passing, ruff clean, mypy clean

## Change History

- [2026-06-08] All items complete. 117 tests passing, ruff clean, mypy clean (43 source files).

## Change History
