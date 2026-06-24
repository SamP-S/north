"""Core task operations: create, read, list, edit, move, delete."""

from pathlib import Path

import frontmatter
import pytest

from north.core import tasks as core
from north.core.errors import Conflict, Invalid, NotFound
from north.core.models import TaskStatus


def test_create_lands_in_draft(board: Path) -> None:
    task = core.create_task(board, "Add login form", agent="opus4.8", labels=["auth"])
    assert task.id == "task-1"
    assert task.status is TaskStatus.DRAFT
    assert task.path.parent.name == "draft"
    assert task.path.name == "task-1 - Add-login-form.md"
    assert task.created_at is not None and task.updated_at is not None


def test_status_mirrored_in_frontmatter(board: Path) -> None:
    task = core.create_task(board, "x")
    meta = frontmatter.load(str(task.path)).metadata
    assert meta["status"] == "draft"
    assert meta["id"] == "task-1"


def test_empty_title_rejected(board: Path) -> None:
    with pytest.raises(Invalid):
        core.create_task(board, "   ")


def test_move_valid_relocates_and_bumps(board: Path) -> None:
    core.create_task(board, "x")
    created = core.get_task(board, "task-1").updated_at
    moved = core.move_task(board, "task-1", "ready")
    assert moved.status is TaskStatus.READY
    assert moved.path.parent.name == "ready"
    assert not (board / "draft" / "task-1 - x.md").exists()
    assert moved.updated_at is not None and created is not None
    assert moved.updated_at >= created


def test_move_illegal_raises(board: Path) -> None:
    core.create_task(board, "x")
    with pytest.raises(Conflict):
        core.move_task(board, "task-1", "done")


def test_move_unknown_status_raises(board: Path) -> None:
    core.create_task(board, "x")
    with pytest.raises(Invalid):
        core.move_task(board, "task-1", "nope")


def test_done_to_ready_reopen_allowed(board: Path) -> None:
    core.create_task(board, "x")
    for status in ("ready", "in_progress", "done", "ready"):
        core.move_task(board, "task-1", status)
    assert core.get_task(board, "task-1").status is TaskStatus.READY


def test_edit_renames_file_and_bumps(board: Path) -> None:
    core.create_task(board, "old title")
    edited = core.edit_task(board, "task-1", title="new title", labels=["a", "b"])
    assert edited.path.name == "task-1 - new-title.md"
    assert not (board / "draft" / "task-1 - old-title.md").exists()
    assert edited.labels == ["a", "b"]


def test_depends_on_roundtrip(board: Path) -> None:
    core.create_task(board, "x", depends_on=["task-9", "task-3"])
    assert core.get_task(board, "task-1").depends_on == ["task-9", "task-3"]


def test_list_filters_by_status(board: Path) -> None:
    core.create_task(board, "a")
    core.create_task(board, "b")
    core.move_task(board, "task-2", "ready")
    ready = core.list_tasks(board, status="ready")
    assert [t.id for t in ready] == ["task-2"]


def test_delete_removes_file(board: Path) -> None:
    core.create_task(board, "x")
    core.delete_task(board, "task-1")
    with pytest.raises(NotFound):
        core.get_task(board, "task-1")
