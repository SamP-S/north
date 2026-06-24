"""Board discovery, scaffolding, and id allocation."""

from pathlib import Path

import pytest

from north.core import board as board_mod
from north.core import tasks as core
from north.core.errors import NotFound
from north.core.models import STATUS_DIRS


def test_init_scaffolds_everything(repo: Path) -> None:
    board = repo / "north"
    assert (board / "config.yml").is_file()
    assert (repo / "AGENTS.md").is_file()
    for name in (*STATUS_DIRS, "archive"):
        assert (board / name).is_dir()
    config = board_mod.load_config(board)
    assert config.mcp_port == 8001
    assert config.auto_commit is False


def test_init_is_idempotent(repo: Path) -> None:
    (repo / "AGENTS.md").write_text("custom", encoding="utf-8")
    board_mod.init_board(repo)  # must not overwrite existing files
    assert (repo / "AGENTS.md").read_text(encoding="utf-8") == "custom"


def test_locate_walks_up(repo: Path) -> None:
    nested = repo / "src" / "deep"
    nested.mkdir(parents=True)
    assert board_mod.locate_board(nested) == repo / "north"


def test_locate_missing_raises(tmp_path: Path) -> None:
    with pytest.raises(NotFound):
        board_mod.locate_board(tmp_path)


def test_next_id_increments(board: Path) -> None:
    assert board_mod.next_id(board) == "task-1"
    core.create_task(board, "one")
    core.create_task(board, "two")
    assert board_mod.next_id(board) == "task-3"
