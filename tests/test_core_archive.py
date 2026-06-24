"""Archive and cleanup behaviour."""

from pathlib import Path

import pytest

from north.core import tasks as core
from north.core.errors import Conflict
from north.core.models import TaskStatus


def _to_done(board: Path, task_id: str) -> None:
    for status in ("ready", "in_progress", "done"):
        core.move_task(board, task_id, status)


def test_archive_moves_off_active_board(board: Path) -> None:
    core.create_task(board, "x")
    archived = core.archive_task(board, "task-1")
    assert archived.archived is True
    assert archived.path.parent.name == "archive"
    assert core.list_tasks(board) == []
    assert [t.id for t in core.list_tasks(board, archived=True)] == ["task-1"]


def test_archived_status_read_from_frontmatter(board: Path) -> None:
    core.create_task(board, "x")
    _to_done(board, "task-1")
    core.archive_task(board, "task-1")
    assert core.get_task(board, "task-1").status is TaskStatus.DONE


def test_cannot_move_archived(board: Path) -> None:
    core.create_task(board, "x")
    core.archive_task(board, "task-1")
    with pytest.raises(Conflict):
        core.move_task(board, "task-1", "ready")


def test_cleanup_archives_done_only(board: Path) -> None:
    core.create_task(board, "done one")
    core.create_task(board, "still draft")
    _to_done(board, "task-1")
    archived = core.cleanup(board)
    assert [t.id for t in archived] == ["task-1"]
    assert {t.id for t in core.list_tasks(board)} == {"task-2"}
