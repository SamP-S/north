"""Service lifecycle commands wrapping ``systemctl --user`` (and linger).

These operate on the OS process/unit, not the app's work loop — ``service stop``
kills the process. ``service status`` reports OS state only (unit + boot/linger);
app health lives behind the top-level ``north status`` command.

Handlers read ``args.unit`` (the ``north`` unit), set via
``set_defaults(unit=...)`` on the ``service`` subparser.
"""

import argparse
import getpass
import subprocess

from north.cli.clients.errors import CLIError

_PAST_TENSE = {
    "start": "started",
    "stop": "stopped",
    "restart": "restarted",
    "enable": "enabled",
    "disable": "disabled",
}


def start(args: argparse.Namespace, _ctx: object) -> int:
    """Start the service via systemctl."""
    return _action(args.unit, "start")


def stop(args: argparse.Namespace, _ctx: object) -> int:
    """Stop the service via systemctl."""
    return _action(args.unit, "stop")


def restart(args: argparse.Namespace, _ctx: object) -> int:
    """Restart the service via systemctl."""
    return _action(args.unit, "restart")


def enable(args: argparse.Namespace, _ctx: object) -> int:
    """Enable the service at boot; ``--now`` also starts it."""
    return _action(args.unit, "enable", now=getattr(args, "now", False))


def disable(args: argparse.Namespace, _ctx: object) -> int:
    """Disable the service at boot; ``--now`` also stops it."""
    return _action(args.unit, "disable", now=getattr(args, "now", False))


def status(args: argparse.Namespace, _ctx: object) -> int:
    """Print OS/process status: systemd unit state plus boot/linger state."""
    for line in _status_lines(args.unit):
        print(line)
    return 0


def _status_lines(unit: str) -> list[str]:
    """Build the OS/process status report lines for ``unit``.

    Degrades gracefully for a stopped/missing unit, since ``_show_properties``
    returns ``{}`` rather than raising in that case.
    """
    props = _show_properties(
        unit,
        ["ActiveState", "SubState", "UnitFileState", "ExecMainStartTimestamp", "NRestarts"],
    )
    active = props.get("ActiveState", "unknown")
    substate = props.get("SubState", "")
    unit_file = props.get("UnitFileState", "") or "not-installed"
    started = props.get("ExecMainStartTimestamp", "")
    restarts = props.get("NRestarts", "")
    linger = _linger_enabled()

    state = f"{active} ({substate})" if substate else active
    lines = [
        unit,
        f"  state:    {state}",
        f"  enabled:  {unit_file}",
    ]
    if started:
        lines.append(f"  started:  {started}")
    if restarts:
        lines.append(f"  restarts: {restarts}")
    lines.append(f"  boot:     {_boot_line(unit_file, linger)}")
    return lines


def _boot_line(unit_file: str, linger: bool | None) -> str:
    """One-line summary of whether the unit will actually start on boot."""
    if linger is None:
        return f"linger unknown (unit {unit_file})"
    if linger and unit_file == "enabled":
        return "linger ON, unit enabled → starts on boot"
    if not linger and unit_file == "enabled":
        return "unit enabled, but linger OFF → will NOT start on boot"
    linger_word = "ON" if linger else "off"
    return f"linger {linger_word}, unit {unit_file} → will NOT start on boot"


def _action(unit: str, action: str, now: bool = False) -> int:
    """Run a state-changing ``systemctl --user`` action against the unit."""
    cmd = [action]
    if now and action in ("enable", "disable"):
        cmd.append("--now")
    cmd.append(unit)
    result = _run_systemctl(cmd)
    output = (result.stdout + result.stderr).strip()
    if output:
        print(output)
    if result.returncode != 0:
        raise CLIError(f"systemctl {action} {unit} failed (exit {result.returncode})")
    print(f"{unit} {_PAST_TENSE[action]}")
    return 0


def _show_properties(unit: str, names: list[str]) -> dict[str, str]:
    """Return selected ``systemctl --user show`` properties as a dict.

    ``show`` exits 0 even for a missing/inactive unit, so this never raises on a
    stopped service — only when systemctl itself is absent.
    """
    result = _run_systemctl(["show", unit, "--property=" + ",".join(names)])
    props: dict[str, str] = {}
    for line in result.stdout.splitlines():
        key, sep, value = line.partition("=")
        if sep:
            props[key] = value
    return props


def _run_systemctl(args: list[str]) -> "subprocess.CompletedProcess[str]":
    """Invoke ``systemctl --user`` with ``args``, raising if systemd is absent."""
    try:
        return subprocess.run(["systemctl", "--user", *args], capture_output=True, text=True)
    except FileNotFoundError as exc:
        raise CLIError("systemctl not found — this host does not appear to use systemd") from exc


def _linger_enabled() -> bool | None:
    """Whether linger is on for the current user, or None if undeterminable."""
    try:
        result = subprocess.run(
            ["loginctl", "show-user", getpass.getuser(), "--property=Linger"],
            capture_output=True,
            text=True,
        )
    except FileNotFoundError:
        return None
    if result.returncode != 0:
        return None
    for line in result.stdout.splitlines():
        if line.startswith("Linger="):
            return line.partition("=")[2].strip() == "yes"
    return None
