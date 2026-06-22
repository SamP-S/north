"""Tests for the unit-parametrized ``service …`` lifecycle CLI commands.

Lifecycle handlers shell out via ``lifecycle.subprocess`` (systemctl/loginctl)
rather than HTTP, so these tests pass ``None`` as the context and monkeypatch
``lifecycle.subprocess.run``.
"""

import argparse
import subprocess

import pytest
from north.cli.clients.errors import CLIError
from north.cli.commands import lifecycle


def _ns(**kwargs: object) -> argparse.Namespace:
    return argparse.Namespace(**kwargs)


def _completed(
    cmd: list[str], rc: int = 0, out: str = "", err: str = ""
) -> "subprocess.CompletedProcess[str]":
    return subprocess.CompletedProcess(cmd, rc, out, err)


UNITS = ("aurora", "borealis")


@pytest.mark.parametrize("unit", UNITS)
def test_start_invokes_systemctl(
    unit: str, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    calls: list[list[str]] = []

    def fake_run(cmd: list[str], **_kw: object) -> "subprocess.CompletedProcess[str]":
        calls.append(cmd)
        return _completed(cmd)

    monkeypatch.setattr(lifecycle.subprocess, "run", fake_run)
    assert lifecycle.start(_ns(unit=unit), None) == 0
    assert calls[0] == ["systemctl", "--user", "start", unit]
    assert f"{unit} started" in capsys.readouterr().out


@pytest.mark.parametrize("unit", UNITS)
def test_stop_invokes_systemctl(unit: str, monkeypatch: pytest.MonkeyPatch) -> None:
    calls: list[list[str]] = []
    monkeypatch.setattr(
        lifecycle.subprocess, "run", lambda cmd, **_k: calls.append(cmd) or _completed(cmd)
    )
    assert lifecycle.stop(_ns(unit=unit), None) == 0
    assert calls[0] == ["systemctl", "--user", "stop", unit]


@pytest.mark.parametrize("unit", UNITS)
def test_restart_uses_correct_verb(unit: str, monkeypatch: pytest.MonkeyPatch) -> None:
    calls: list[list[str]] = []
    monkeypatch.setattr(
        lifecycle.subprocess, "run", lambda cmd, **_k: calls.append(cmd) or _completed(cmd)
    )
    lifecycle.restart(_ns(unit=unit), None)
    assert calls[0] == ["systemctl", "--user", "restart", unit]


@pytest.mark.parametrize("unit", UNITS)
def test_enable_now_appends_flag(unit: str, monkeypatch: pytest.MonkeyPatch) -> None:
    calls: list[list[str]] = []
    monkeypatch.setattr(
        lifecycle.subprocess, "run", lambda cmd, **_k: calls.append(cmd) or _completed(cmd)
    )
    lifecycle.enable(_ns(unit=unit, now=True), None)
    assert calls[0] == ["systemctl", "--user", "enable", "--now", unit]


@pytest.mark.parametrize("unit", UNITS)
def test_enable_without_now_omits_flag(unit: str, monkeypatch: pytest.MonkeyPatch) -> None:
    calls: list[list[str]] = []
    monkeypatch.setattr(
        lifecycle.subprocess, "run", lambda cmd, **_k: calls.append(cmd) or _completed(cmd)
    )
    lifecycle.enable(_ns(unit=unit, now=False), None)
    assert calls[0] == ["systemctl", "--user", "enable", unit]


@pytest.mark.parametrize("unit", UNITS)
def test_disable_now_appends_flag(unit: str, monkeypatch: pytest.MonkeyPatch) -> None:
    calls: list[list[str]] = []
    monkeypatch.setattr(
        lifecycle.subprocess, "run", lambda cmd, **_k: calls.append(cmd) or _completed(cmd)
    )
    lifecycle.disable(_ns(unit=unit, now=True), None)
    assert calls[0] == ["systemctl", "--user", "disable", "--now", unit]


@pytest.mark.parametrize("unit", UNITS)
def test_disable_without_now_omits_flag(unit: str, monkeypatch: pytest.MonkeyPatch) -> None:
    calls: list[list[str]] = []
    monkeypatch.setattr(
        lifecycle.subprocess, "run", lambda cmd, **_k: calls.append(cmd) or _completed(cmd)
    )
    lifecycle.disable(_ns(unit=unit, now=False), None)
    assert calls[0] == ["systemctl", "--user", "disable", unit]


@pytest.mark.parametrize("unit", UNITS)
def test_action_nonzero_exit_raises(unit: str, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(
        lifecycle.subprocess, "run", lambda cmd, **_k: _completed(cmd, rc=1, err="boom")
    )
    with pytest.raises(CLIError) as exc:
        lifecycle.stop(_ns(unit=unit), None)
    assert f"stop {unit} failed" in str(exc.value)


@pytest.mark.parametrize("unit", UNITS)
def test_systemctl_missing_raises(unit: str, monkeypatch: pytest.MonkeyPatch) -> None:
    def fake_run(_cmd: list[str], **_kw: object) -> "subprocess.CompletedProcess[str]":
        raise FileNotFoundError

    monkeypatch.setattr(lifecycle.subprocess, "run", fake_run)
    with pytest.raises(CLIError) as exc:
        lifecycle.start(_ns(unit=unit), None)
    assert "systemctl not found" in str(exc.value)


def _status_fake(show: str, linger: str = "Linger=yes\n", linger_rc: int = 0):
    def fake_run(cmd: list[str], **_kw: object) -> "subprocess.CompletedProcess[str]":
        if cmd[0] == "systemctl":
            return _completed(cmd, out=show)
        if cmd[0] == "loginctl":
            return _completed(cmd, rc=linger_rc, out=linger)
        return _completed(cmd)

    return fake_run


@pytest.mark.parametrize("unit", UNITS)
def test_status_running_enabled_linger(
    unit: str, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    show = (
        "ActiveState=active\nSubState=running\nUnitFileState=enabled\n"
        "ExecMainStartTimestamp=Sun 2026-06-15 11:23:34 UTC\nNRestarts=0\n"
    )
    monkeypatch.setattr(lifecycle.subprocess, "run", _status_fake(show))
    assert lifecycle.status(_ns(unit=unit), None) == 0
    out = capsys.readouterr().out
    assert unit in out
    assert "active (running)" in out
    assert "enabled" in out
    assert "starts on boot" in out


@pytest.mark.parametrize("unit", UNITS)
def test_status_enabled_but_no_linger_warns(
    unit: str, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    show = "ActiveState=active\nSubState=running\nUnitFileState=enabled\nNRestarts=0\n"
    monkeypatch.setattr(lifecycle.subprocess, "run", _status_fake(show, linger="Linger=no\n"))
    lifecycle.status(_ns(unit=unit), None)
    out = capsys.readouterr().out
    assert "linger OFF" in out
    assert "will NOT start on boot" in out


@pytest.mark.parametrize("unit", UNITS)
def test_status_dead_unit(
    unit: str, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    show = "ActiveState=inactive\nSubState=dead\nUnitFileState=disabled\n"
    monkeypatch.setattr(lifecycle.subprocess, "run", _status_fake(show))
    lifecycle.status(_ns(unit=unit), None)
    out = capsys.readouterr().out
    assert "inactive (dead)" in out
    assert "will NOT start on boot" in out
