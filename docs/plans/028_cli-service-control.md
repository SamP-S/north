# 028 — CLI service control (`<svc> service …`)

> **Superseded by [031 — Unified North CLI](031_unified-north-cli.md).** The
> per-service `aurora service …` / `borealis service …` surface described here is
> now `north service aurora …` / `north service borealis …` (plus a new
> `north service status` aggregate), implemented via one unit-parametrized
> `lifecycle.py`. Retained for historical context only.

## Context

North's two services (aurora, borealis) run as **systemd user units** with
linger enabled, so reboot-persistence is already handled by `scripts/install.sh`
(installs units, `loginctl enable-linger`, `systemctl --user enable --now`). The
gap is the **CLI control surface**: each CLI's `lifecycle.py` only wraps
`systemctl --user start/stop` and exposes them as bare top-level commands
(`aurora start`). There is no `restart`/`enable`/`disable`/`status`, and process
lifecycle verbs sit at the same level as app-level commands, which blurs the
line between "the OS process" and "the app's work loop".

This plan nests all OS-process lifecycle verbs under a `service` subcommand in
**both** CLIs, and adds an OS-level `service status`. Control stays per-CLI (no
unified `north` CLI) — two twins are cheap to maintain.

## Scope

Per-CLI, mirrored for `aurora` and `borealis`:

```
<svc> status                                  # app health (/api/status) — UNCHANGED
<svc> pause | resume                          # app work-loop control — UNCHANGED, stays top-level
<svc> queue | logs | review | …               # observation/domain — UNCHANGED
<svc> service start | stop | restart | enable | disable | status   # OS lifecycle — NEW group
```

Key boundaries (the reason for the reorg):
- **`service stop`** kills the process; **`pause`** keeps it running but halts the
  supervisor loop. Different concepts → different command groups.
- **Top-level `status`** = app health (existing `/api/status`). **`service status`**
  = OS/process status only (systemd unit + boot/linger). The `service` prefix
  disambiguates, so both can coexist; `service status` does **not** duplicate app
  health.

`service status` reports:
- systemd unit: `ActiveState`/`SubState` (e.g. `active (running)`), `UnitFileState`
  (`enabled`/`disabled`), uptime (from `ExecMainStartTimestamp`), `NRestarts`.
- boot persistence: linger state (`loginctl show-user <user> --property=Linger`),
  with a warning when a unit is `enabled` but linger is off (won't start on boot).
- Degrades gracefully when the unit is down (print systemd/boot state; no app call —
  `service status` is OS-only by design).

`enable`/`disable` take an optional `--now` (enable+start / disable+stop), matching
`systemctl` semantics.

## Files

Pure CLI-surface reorg; **no service-side changes**. Aurora and borealis are
independent twins (no shared lib) — mirror each edit in both.

- `aurora/aurora/cli/commands/lifecycle.py` (+ `borealis/.../lifecycle.py`):
  - Generalise `_systemctl(action)` to support `start|stop|restart|enable|disable`
    (incl. `--now` passthrough for enable/disable).
  - Add `status()` — queries `systemctl --user show <unit> --property=…` and
    `loginctl show-user --property=Linger`, formats the block, degrades gracefully.
    Keep the existing `CLIError` on `systemctl`/`loginctl` not found.
- `aurora/aurora/cli/main.py` (+ `borealis/.../main.py`):
  - Replace the top-level `start`/`stop` subparsers with a `service` subparser that
    has its own `add_subparsers` for `start|stop|restart|enable|disable|status`.
    Each sets `func=lifecycle.<verb>, needs_client=False`.
  - Handle `<svc> service` with no subcommand (print service help, return 1),
    mirroring the top-level no-command behaviour.
- `aurora/aurora/cli/client.py` (+ borealis twin): connect-error hint updated from
  `try \`aurora start\`` to `try \`aurora service start\``.
- `aurora/tests/test_cli_lifecycle.py` + `borealis/tests/test_cli_lifecycle.py` (new):
  - Mock `subprocess.run` to assert each verb calls the right `systemctl --user …`
    argv and maps exit codes to `CLIError`.
  - Test `service status` parsing/formatting (running+enabled+linger; dead+disabled;
    enabled-but-no-linger warning) with mocked `systemctl show` / `loginctl` output.

## Todos

- [x] 1. aurora `lifecycle.py`: generalise `_systemctl`, add `restart`/`enable`/`disable` (+`--now`)
- [x] 2. aurora `lifecycle.py`: add `status()` (systemd show + linger, graceful degrade)
- [x] 3. aurora `main.py`: nest verbs under a `service` subparser; handle bare `service`
- [x] 4. Mirror 1–3 in borealis `lifecycle.py` + `main.py`
- [x] 5. Add `test_cli_lifecycle.py` for both packages (argv mapping + status formatting)
- [x] 6. Lint/format/mypy; run both CLI test suites
- [x] 7. Live smoke: `aurora/borealis service status` run live (exercises the real
       `systemctl`/`loginctl` path + degraded formatting). Start/stop round-trip
       deferred — this box has no installed units and runs via nohup (see plan 027).

## Verification

- `uv run pytest aurora/tests borealis/tests` green; `ruff check` clean; `mypy` no
  net-new errors.
- Live (units must be the systemd ones, not the ad-hoc nohup procs — see plan 027):
  - `aurora service status` shows `active (running)`, `enabled`, linger ON.
  - `aurora service restart` round-trips; `aurora status` still reports app health.
  - `aurora service disable` flips `UnitFileState` to `disabled`; `enable` restores it.

## Change History

- [2026-06-15] Plan created. Per-CLI `service` subcommand group; top-level `status`
  stays app-only, `service status` is OS-only (systemd + linger). No service-side
  changes.
- [2026-06-15] Implemented in both CLIs: `service start|stop|restart|enable|disable|status`
  (enable/disable take `--now`), bare `service` prints help. `status` reads
  `systemctl --user show` + `loginctl … Linger` and degrades gracefully. Connect-error
  hints updated. New `test_cli_lifecycle.py` for both packages; full suites green
  (294 passed), ruff + mypy clean. Live-smoked `service status` on this box (units not
  installed here → reports `not-installed`/`won't start on boot`, as expected).
