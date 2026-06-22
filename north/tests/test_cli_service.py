"""Tests for the aggregate ``service status`` command covering both units.

``aggregate_status`` calls ``lifecycle._status_lines`` for both the aurora and
borealis units, which shells out via ``lifecycle.subprocess``; we monkeypatch
that and pass ``None`` as the context.
"""

import argparse
import subprocess

import pytest
from north.cli.commands import lifecycle, service


def _completed(
    cmd: list[str], rc: int = 0, out: str = "", err: str = ""
) -> "subprocess.CompletedProcess[str]":
    return subprocess.CompletedProcess(cmd, rc, out, err)


def _status_fake(show: str, linger: str = "Linger=yes\n"):
    def fake_run(cmd: list[str], **_kw: object) -> "subprocess.CompletedProcess[str]":
        if cmd[0] == "systemctl":
            return _completed(cmd, out=show)
        if cmd[0] == "loginctl":
            return _completed(cmd, out=linger)
        return _completed(cmd)

    return fake_run


def test_aggregate_status_reports_both_units(
    monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    show = (
        "ActiveState=active\nSubState=running\nUnitFileState=enabled\nNRestarts=0\n"
    )
    monkeypatch.setattr(lifecycle.subprocess, "run", _status_fake(show))

    rc = service.aggregate_status(argparse.Namespace(), None)
    out = capsys.readouterr().out
    assert rc == 0
    assert "aurora" in out
    assert "borealis" in out
    assert "active (running)" in out
    # both unit headings rendered (one block per unit)
    assert out.count("state:") == 2
