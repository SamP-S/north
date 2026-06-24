"""`north task …` — create, view, list, edit, move, archive, delete tasks."""

import argparse
from pathlib import Path

from north.cli.errors import CLIError
from north.cli.prompts import confirm
from north.cli.render import render_task, render_task_list
from north.core import tasks as core
from north.core.board import locate_board


def _read_body(args: argparse.Namespace) -> str | None:
    """Return the body from --body / --body-file, or None if neither given."""
    if getattr(args, "body_file", None):
        path = Path(args.body_file)
        if not path.is_file():
            raise CLIError(f"body file not found: {path}")
        return path.read_text(encoding="utf-8")
    return getattr(args, "body", None)


def create(args: argparse.Namespace) -> int:
    board = locate_board()
    task = core.create_task(
        board,
        args.title,
        agent=args.agent or "",
        labels=args.labels or [],
        depends_on=args.depends_on or [],
        body=_read_body(args) or "",
    )
    print(f"Created {task.id} ({task.status}): {task.title}")
    return 0


def view(args: argparse.Namespace) -> int:
    task = core.get_task(locate_board(), args.task_id)
    print(render_task(task, plain=args.plain, as_json=args.json))
    return 0


def list_(args: argparse.Namespace) -> int:
    tasks = core.list_tasks(locate_board(), status=args.status, archived=args.archived)
    print(render_task_list(tasks, plain=args.plain, as_json=args.json))
    return 0


def edit(args: argparse.Namespace) -> int:
    task = core.edit_task(
        locate_board(),
        args.task_id,
        title=args.title,
        agent=args.agent,
        labels=args.labels,
        depends_on=args.depends_on,
        body=_read_body(args),
    )
    print(f"Edited {task.id}")
    return 0


def move(args: argparse.Namespace) -> int:
    task = core.move_task(locate_board(), args.task_id, args.status)
    print(f"{task.id} → {task.status}")
    return 0


def archive(args: argparse.Namespace) -> int:
    task = core.archive_task(locate_board(), args.task_id)
    print(f"Archived {task.id}")
    return 0


def delete(args: argparse.Namespace) -> int:
    board = locate_board()
    task = core.get_task(board, args.task_id)
    if not args.yes and not confirm(f"Delete {task.id} ({task.title})?"):
        print("Aborted.")
        return 1
    core.delete_task(board, args.task_id)
    print(f"Deleted {task.id}")
    return 0
