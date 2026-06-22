"""Tests for the status commands, including combined partial-failure handling."""

import httpx
import pytest
from north.cli.commands import status


def test_aurora_block_renders(
    ns, make_ctx, capsys: pytest.CaptureFixture[str]
) -> None:
    def aurora_handler(_request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={"runner_state": "running", "oauth_health": "ok"})

    rc = status.aurora(ns(), make_ctx(aurora=aurora_handler))
    out = capsys.readouterr().out
    assert rc == 0
    assert "runner state:   running" in out
    assert "oauth health:   ok" in out


def test_borealis_block_renders(
    ns, make_ctx, capsys: pytest.CaptureFixture[str]
) -> None:
    def borealis_handler(_request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={"runner_state": "idle", "board_loaded": True})

    rc = status.borealis(ns(), make_ctx(borealis=borealis_handler))
    out = capsys.readouterr().out
    assert rc == 0
    assert "board loaded:  True" in out


def test_combined_both_ok(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    def ok(_request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={"runner_state": "running", "board_loaded": True})

    rc = status.status(ns(), make_ctx(aurora=ok, borealis=ok))
    out = capsys.readouterr().out
    assert rc == 0
    assert "aurora" in out
    assert "borealis" in out
    assert "error:" not in out


def test_combined_partial_failure(
    ns, make_ctx, capsys: pytest.CaptureFixture[str]
) -> None:
    def aurora_ok(_request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={"runner_state": "running"})

    def borealis_down(_request: httpx.Request) -> httpx.Response:
        return httpx.Response(503, json={"detail": "unavailable"})

    rc = status.status(ns(), make_ctx(aurora=aurora_ok, borealis=borealis_down))
    out = capsys.readouterr().out
    assert rc == 1
    # aurora block still rendered despite borealis failing
    assert "runner state:   running" in out
    assert "  error:" in out
