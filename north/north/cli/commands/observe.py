"""Observation commands: logs, queue."""

import argparse
from typing import Any

from north.cli.context import NorthContext


def queue(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Print the active task queue (in-progress, then eligible queued tasks)."""
    params = {"project": args.project} if args.project else None
    tasks: list[dict[str, Any]] = ctx.borealis.get("/api/queue", params=params)
    if not tasks:
        print("queue is empty")
        return 0
    for task in tasks:
        print(
            f"{task.get('task_id')}  [{task.get('status')}]  "
            f"{task.get('project')}/{task.get('feature')}  {task.get('title')}"
        )
    return 0


def logs(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Stream ``agent.output`` events until interrupted with Ctrl+C."""
    project = getattr(args, "project", None)
    try:
        for event in ctx.aurora.sse_stream("/api/events"):
            if event.get("type") != "agent.output":
                continue
            if project and event.get("project") != project:
                continue
            prefix = (
                f"[{event.get('project')}/{event.get('feature')}/"
                f"{event.get('task_id')}]"
            )
            print(f"{prefix} {event.get('output', '')}")
    except KeyboardInterrupt:
        print()
    return 0
