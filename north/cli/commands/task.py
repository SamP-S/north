"""Task management commands: create, show, list, status, delete, promote, split."""

import argparse
import json
from pathlib import Path
from typing import Any

from north.cli.clients.errors import CLIError
from north.cli.context import NorthContext
from north.cli.prompts import confirm


def create(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Create a new task on a feature."""
    if args.body and args.body_file:
        raise CLIError("--body and --body-file are mutually exclusive")
    body_text = args.body or ""
    if args.body_file:
        path = Path(args.body_file)
        if not path.is_file():
            raise CLIError(f"body file not found: {args.body_file}")
        body_text = path.read_text(encoding="utf-8")

    body: dict[str, Any] = {
        "title": args.title,
        "pipeline": args.pipeline,
        "body": body_text,
        "depends_on": args.depends_on or [],
    }
    data: dict[str, Any] = ctx.board.post(
        f"/api/projects/{args.project}/features/{args.feature}/tasks", body=body
    )
    print(f"created task: {args.project}/{args.feature}/{data.get('task_id')}")
    return 0


def show(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Show a single task, including its result content if present."""
    data: dict[str, Any] = ctx.board.get(
        f"/api/projects/{args.project}/features/{args.feature}/tasks/{args.task_id}"
    )
    for key in (
        "task_id",
        "title",
        "status",
        "pipeline",
        "project",
        "feature",
        "depends_on",
        "created_at",
        "ready_at",
    ):
        print(f"{key + ':':13} {data.get(key)}")
    body = data.get("body")
    if body:
        print(f"\n{body}")
    result_content = data.get("result_content")
    if result_content:
        print(f"\n--- result ---\n{result_content}")
    return 0


def list_(args: argparse.Namespace, ctx: NorthContext) -> int:
    """List a feature's tasks, optionally filtered by ``--status``."""
    items: list[dict[str, Any]] = ctx.board.get(
        f"/api/projects/{args.project}/features/{args.feature}/tasks"
    )
    if args.status:
        items = [t for t in items if t.get("status") == args.status]
    if not items:
        print("no tasks")
        return 0
    for task in items:
        deps = ",".join(task.get("depends_on") or [])
        deps_part = f"  (deps: {deps})" if deps else ""
        print(
            f"{task.get('task_id')}  [{task.get('status')}]  "
            f"{task.get('title')}{deps_part}"
        )
    return 0


def edit(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Edit a task's fields; unspecified flags keep their current values."""
    if args.body and args.body_file:
        raise CLIError("--body and --body-file are mutually exclusive")
    current: dict[str, Any] = ctx.board.get(
        f"/api/projects/{args.project}/features/{args.feature}/tasks/{args.task_id}"
    )
    if args.body_file:
        path = Path(args.body_file)
        if not path.is_file():
            raise CLIError(f"body file not found: {args.body_file}")
        body_text: str | None = path.read_text(encoding="utf-8")
    else:
        body_text = args.body
    body: dict[str, Any] = {
        "title": args.title if args.title is not None else current.get("title"),
        "pipeline": (
            args.pipeline if args.pipeline is not None else current.get("pipeline")
        ),
        "body": body_text if body_text is not None else (current.get("body") or ""),
        "depends_on": (
            args.depends_on
            if args.depends_on is not None
            else (current.get("depends_on") or [])
        ),
        "status": args.status if args.status is not None else current.get("status"),
    }
    ctx.board.put(
        f"/api/projects/{args.project}/features/{args.feature}/tasks/{args.task_id}",
        body=body,
    )
    print(f"updated task: {args.project}/{args.feature}/{args.task_id}")
    return 0


def status(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Set a task's status."""
    data: dict[str, Any] = ctx.board.patch(
        f"/api/projects/{args.project}/features/{args.feature}"
        f"/tasks/{args.task_id}/status",
        body={"status": args.status},
    )
    print(
        f"{args.project}/{args.feature}/{args.task_id} → "
        f"{data.get('status', args.status)}"
    )
    return 0


def delete(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Delete a task and its result file."""
    if not args.yes and not confirm(
        f"Delete task {args.project}/{args.feature}/{args.task_id}?"
    ):
        print("aborted")
        return 0
    ctx.board.delete(
        f"/api/projects/{args.project}/features/{args.feature}/tasks/{args.task_id}"
    )
    print(f"deleted task: {args.project}/{args.feature}/{args.task_id}")
    return 0


def promote(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Promote a draft task to ready."""
    ctx.board.post(
        f"/api/projects/{args.project}/features/{args.feature}"
        f"/tasks/{args.task_id}/promote"
    )
    print(f"promoted task: {args.project}/{args.feature}/{args.task_id} → ready")
    return 0


def split(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Split a task into replacement children from a JSON task list."""
    if bool(args.tasks_json) == bool(args.tasks_file):
        raise CLIError("exactly one of --tasks-json or --tasks-file is required")
    raw = args.tasks_json or ""
    if args.tasks_file:
        path = Path(args.tasks_file)
        if not path.is_file():
            raise CLIError(f"tasks file not found: {args.tasks_file}")
        raw = path.read_text(encoding="utf-8")
    try:
        tasks = json.loads(raw)
    except json.JSONDecodeError as exc:
        raise CLIError(f"invalid tasks JSON: {exc}")
    data: dict[str, Any] = ctx.board.post(
        f"/api/projects/{args.project}/features/{args.feature}"
        f"/tasks/{args.task_id}/split",
        body={"tasks": tasks},
    )
    created = ", ".join(data.get("created") or [])
    print(f"split task {args.task_id} → {created} (parent superseded)")
    return 0
