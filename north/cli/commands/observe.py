"""Observation commands: queue."""

import argparse
from typing import Any

from north.cli.context import NorthContext


def queue(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Print the active task queue (in-progress, then eligible queued tasks)."""
    params = {"project": args.project} if args.project else None
    tasks: list[dict[str, Any]] = ctx.board.get("/api/queue", params=params)
    if not tasks:
        print("queue is empty")
        return 0
    for task in tasks:
        print(
            f"{task.get('task_id')}  [{task.get('status')}]  "
            f"{task.get('project')}/{task.get('feature')}  {task.get('title')}"
        )
    return 0
