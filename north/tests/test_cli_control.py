"""Tests for the pause / resume control commands."""

import httpx
import pytest
from north.cli.commands import control


def test_pause_posts_action(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    seen: dict[str, object] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        seen["method"] = request.method
        seen["path"] = request.url.path
        return httpx.Response(200, json={"message": "runner paused"})

    rc = control.pause(ns(), make_ctx(aurora=handler))
    out = capsys.readouterr().out
    assert rc == 0
    assert seen == {"method": "POST", "path": "/api/control"}
    assert "runner paused" in out


def test_resume_reports_message(
    ns, make_ctx, capsys: pytest.CaptureFixture[str]
) -> None:
    def handler(_request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={"message": "already running"})

    rc = control.resume(ns(), make_ctx(aurora=handler))
    out = capsys.readouterr().out
    assert rc == 0
    assert "already running" in out
