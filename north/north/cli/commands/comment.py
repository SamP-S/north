"""Comment commands: add to / list a task or feature thread."""

import argparse
from typing import Any

from north.cli.context import NorthContext


def _thread_url(args: argparse.Namespace) -> str:
    base = f"/api/projects/{args.project}/features/{args.feature}"
    if getattr(args, "task_id", None):
        return f"{base}/tasks/{args.task_id}/comments"
    return f"{base}/comments"


def add(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Append a typed comment to a task or feature thread."""
    data: dict[str, Any] = ctx.borealis.post(
        _thread_url(args),
        body={"kind": args.kind, "author": args.author, "text": args.text},
    )
    status = data.get("task_status")
    print(f"comment added{f' (task → {status})' if status else ''}")
    return 0


def list_(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Print a task or feature thread."""
    data: list[dict[str, Any]] = ctx.borealis.get(_thread_url(args))
    if not data:
        print("no comments")
        return 0
    for entry in data:
        print(f"[{entry.get('kind')}] {entry.get('author')} — {entry.get('at')}")
        print(f"{entry.get('text')}\n")
    return 0
