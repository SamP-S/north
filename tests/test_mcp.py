"""MCP surface: single server, task tools wired to core."""

import asyncio
from pathlib import Path

import pytest

from north.service import mcp


def test_build_server_tools() -> None:
    server = mcp.build_server()
    names = {t.name for t in asyncio.run(server.list_tools())}
    assert names == {"list_tasks", "get_task", "create_task", "set_task_status", "edit_task"}


def test_tools_drive_the_board(board: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("NORTH_BOARD", str(board))
    created = mcp.create_task("From MCP", agent="aurora:pipe")
    assert created["id"] == "task-1"
    assert mcp.get_task("task-1")["agent"] == "aurora:pipe"
    listed = mcp.list_tasks()
    assert listed[0]["id"] == "task-1"
    assert "body" not in listed[0]
    moved = mcp.set_task_status("task-1", "ready")
    assert moved["status"] == "ready"


def test_board_errors_become_tool_errors(board: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("NORTH_BOARD", str(board))
    mcp.create_task("x")
    with pytest.raises(ValueError, match="conflict"):
        mcp.set_task_status("task-1", "done")
