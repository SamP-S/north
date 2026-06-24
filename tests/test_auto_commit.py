"""auto_commit makes a local commit per mutation (and never pushes)."""

from pathlib import Path

import git

from north.core import board as board_mod
from north.core import tasks as core


def test_auto_commit_creates_commit(tmp_path: Path) -> None:
    repo = git.Repo.init(tmp_path)
    repo.config_writer().set_value("user", "name", "t").release()
    repo.config_writer().set_value("user", "email", "t@t").release()
    board = board_mod.init_board(tmp_path)
    board_mod.write_config(board, board_mod.Config(auto_commit=True))

    core.create_task(board, "committed task")

    messages = [c.message for c in repo.iter_commits()]
    assert any(m.startswith("north: create task-1") for m in messages)


def test_no_commit_when_disabled(tmp_path: Path) -> None:
    repo = git.Repo.init(tmp_path)
    repo.config_writer().set_value("user", "name", "t").release()
    repo.config_writer().set_value("user", "email", "t@t").release()
    board = board_mod.init_board(tmp_path)  # auto_commit defaults to False

    core.create_task(board, "x")

    assert repo.head.is_valid() is False  # nothing committed
