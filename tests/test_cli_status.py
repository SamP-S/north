"""Tests for the board status command."""

import httpx
import pytest

from north.cli.clients.errors import CLIError
from north.cli.commands import status


def test_board_block_renders(
    ns, make_ctx, capsys: pytest.CaptureFixture[str]
) -> None:
    def board_handler(_request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={"runner_state": "idle", "board_loaded": True})

    rc = status.status(ns(), make_ctx(board=board_handler))
    out = capsys.readouterr().out
    assert rc == 0
    assert "runner state:  idle" in out
    assert "board loaded:  True" in out


def test_board_fetch_failure_raises(ns, make_ctx) -> None:
    def board_down(_request: httpx.Request) -> httpx.Response:
        return httpx.Response(503, json={"detail": "unavailable"})

    with pytest.raises(CLIError):
        status.status(ns(), make_ctx(board=board_down))
