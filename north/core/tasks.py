"""Task operations over the board: create, read, list, edit, move, archive, delete.

Every task is one Markdown file. Its status is the folder it lives in; the
``status`` frontmatter key is kept in sync. Each mutation optionally makes a
local git commit when ``auto_commit`` is set.
"""

from datetime import UTC, datetime, timedelta
from pathlib import Path
from typing import Any, cast

import frontmatter
import yaml

from north.core import git
from north.core.board import (
    ARCHIVE_DIR,
    load_config,
    next_id,
    task_filename,
    task_files,
)
from north.core.errors import Conflict, Invalid, NotFound
from north.core.models import STATUS_DIRS, TRANSITIONS, Task, TaskStatus


def _now() -> datetime:
    return datetime.now(UTC)


def _parse_dt(value: object) -> datetime | None:
    if not value:
        return None
    try:
        return datetime.fromisoformat(str(value))
    except (ValueError, TypeError):
        return None


def parse_status(value: str | TaskStatus) -> TaskStatus:
    """Coerce a string to a :class:`TaskStatus`, raising :class:`Invalid`."""
    try:
        return TaskStatus(str(value))
    except ValueError:
        allowed = ", ".join(STATUS_DIRS)
        raise Invalid(f"unknown status {value!r} (expected one of: {allowed})") from None


# --- load / persist --------------------------------------------------------------


def _load_task(path: Path) -> Task:
    try:
        post = frontmatter.load(str(path))
        meta: dict[str, Any] = cast(dict[str, Any], post.metadata)
        archived = path.parent.name == ARCHIVE_DIR
        status = parse_status(meta["status"]) if archived else parse_status(path.parent.name)
        return Task(
            id=str(meta["id"]),
            title=str(meta["title"]),
            status=status,
            path=path,
            agent=str(meta.get("agent") or ""),
            labels=[str(label) for label in meta.get("labels", [])],
            depends_on=[str(dep) for dep in meta.get("depends_on", [])],
            created_at=_parse_dt(meta.get("created_at")),
            updated_at=_parse_dt(meta.get("updated_at")),
            body=post.content.strip(),
            archived=archived,
        )
    except Invalid:
        raise
    except Exception as exc:
        raise Invalid(f"failed to parse {path.name}: {exc}") from exc


def _target_path(board: Path, task: Task) -> Path:
    folder = board / (ARCHIVE_DIR if task.archived else task.status.value)
    return folder / task_filename(task.id, task.title)


def _render(task: Task) -> str:
    meta = {
        "id": task.id,
        "title": task.title,
        "status": str(task.status),
        "agent": task.agent,
        "labels": task.labels,
        "depends_on": task.depends_on,
        "created_at": task.created_at.isoformat() if task.created_at else None,
        "updated_at": task.updated_at.isoformat() if task.updated_at else None,
    }
    front = yaml.safe_dump(meta, default_flow_style=False, sort_keys=False).rstrip()
    body = task.body.strip()
    return f"---\n{front}\n---\n\n{body}\n" if body else f"---\n{front}\n---\n"


def _save(board: Path, task: Task, *, old_path: Path | None, message: str) -> Task:
    target = _target_path(board, task)
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(_render(task), encoding="utf-8")
    removed: list[Path] = []
    if old_path is not None and old_path.resolve() != target.resolve() and old_path.exists():
        old_path.unlink()
        removed.append(old_path)
    task.path = target
    _commit(board, [target], removed, message)
    return task


def _commit(board: Path, paths: list[Path], removed: list[Path], message: str) -> None:
    if load_config(board).auto_commit:
        git.commit_board(board, message, paths, removed=removed)


def _find(board: Path, task_id: str) -> Task:
    prefix = f"{task_id} - "
    for path in task_files(board, include_archive=True):
        if path.name.startswith(prefix):
            task = _load_task(path)
            if task.id == task_id:
                return task
    raise NotFound(f"task {task_id!r} not found")


# --- public operations -----------------------------------------------------------


def create_task(
    board: Path,
    title: str,
    *,
    agent: str = "",
    labels: list[str] | None = None,
    depends_on: list[str] | None = None,
    body: str = "",
) -> Task:
    """Create a task in ``draft/``."""
    if not title.strip():
        raise Invalid("task title must not be empty")
    now = _now()
    task = Task(
        id=next_id(board),
        title=title.strip(),
        status=TaskStatus.DRAFT,
        path=board,  # replaced by _save
        agent=agent,
        labels=labels or [],
        depends_on=depends_on or [],
        created_at=now,
        updated_at=now,
        body=body,
    )
    return _save(board, task, old_path=None, message=f"north: create {task.id}")


def get_task(board: Path, task_id: str) -> Task:
    """Return one task by id (searches all folders incl. archive)."""
    return _find(board, task_id)


def list_tasks(
    board: Path, *, status: str | None = None, archived: bool = False
) -> list[Task]:
    """List active tasks (add archived ones with ``archived=True``)."""
    wanted = parse_status(status) if status else None
    tasks = [_load_task(p) for p in task_files(board, include_archive=archived)]
    if wanted is not None:
        tasks = [t for t in tasks if t.status == wanted]
    tasks.sort(key=lambda t: (t.archived, _id_num(t.id)))
    return tasks


def edit_task(
    board: Path,
    task_id: str,
    *,
    title: str | None = None,
    agent: str | None = None,
    labels: list[str] | None = None,
    depends_on: list[str] | None = None,
    body: str | None = None,
) -> Task:
    """Edit a task's fields/body. ``updated_at`` is bumped."""
    task = _find(board, task_id)
    old_path = task.path
    if title is not None:
        if not title.strip():
            raise Invalid("task title must not be empty")
        task.title = title.strip()
    if agent is not None:
        task.agent = agent
    if labels is not None:
        task.labels = labels
    if depends_on is not None:
        task.depends_on = depends_on
    if body is not None:
        task.body = body
    task.updated_at = _now()
    return _save(board, task, old_path=old_path, message=f"north: edit {task.id}")


def move_task(board: Path, task_id: str, new_status: str | TaskStatus) -> Task:
    """Change a task's status (validates the transition; moves the file)."""
    target = parse_status(new_status)
    task = _find(board, task_id)
    if task.archived:
        raise Conflict(f"task {task_id!r} is archived; cannot change its status")
    if target == task.status:
        return task
    if target not in TRANSITIONS[task.status]:
        allowed = ", ".join(sorted(s.value for s in TRANSITIONS[task.status])) or "(none)"
        raise Conflict(
            f"illegal transition {task.status} → {target} "
            f"(from {task.status} you can go to: {allowed})"
        )
    old_path = task.path
    task.status = target
    task.updated_at = _now()
    return _save(board, task, old_path=old_path, message=f"north: {task.id} → {target}")


def archive_task(board: Path, task_id: str) -> Task:
    """Move a task into ``archive/`` (off the active board)."""
    task = _find(board, task_id)
    if task.archived:
        raise Conflict(f"task {task_id!r} is already archived")
    old_path = task.path
    task.archived = True
    task.updated_at = _now()
    return _save(board, task, old_path=old_path, message=f"north: archive {task.id}")


def cleanup(board: Path, *, older_than_days: int | None = None) -> list[Task]:
    """Archive all ``done/`` tasks (optionally only those older than N days)."""
    cutoff = _now() - timedelta(days=older_than_days) if older_than_days else None
    archived: list[Task] = []
    for task in list_tasks(board, status=str(TaskStatus.DONE)):
        if cutoff is not None and (task.updated_at is None or task.updated_at > cutoff):
            continue
        archived.append(archive_task(board, task.id))
    return archived


def delete_task(board: Path, task_id: str) -> None:
    """Delete a task file."""
    task = _find(board, task_id)
    task.path.unlink()
    _commit(board, [], [task.path], f"north: delete {task.id}")


def status_counts(board: Path) -> dict[str, int]:
    """Counts of active tasks per status, in board order (for `north board`)."""
    counts = {status: 0 for status in STATUS_DIRS}
    for task in list_tasks(board):
        counts[task.status.value] += 1
    return counts


def _id_num(task_id: str) -> int:
    try:
        return int(task_id.removeprefix("task-"))
    except ValueError:
        return 0
