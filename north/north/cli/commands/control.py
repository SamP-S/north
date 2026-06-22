"""Runtime control commands: pause and resume."""

import argparse
from typing import Any

from north.cli.context import NorthContext


def pause(_args: argparse.Namespace, ctx: NorthContext) -> int:
    """Pause the runner; already-paused responses are reported as-is."""
    data: dict[str, Any] = ctx.aurora.post("/api/control", body={"action": "pause"})
    print(data.get("message", "runner paused"))
    return 0


def resume(_args: argparse.Namespace, ctx: NorthContext) -> int:
    """Resume the runner; not-paused responses are reported as-is."""
    data: dict[str, Any] = ctx.aurora.post("/api/control", body={"action": "resume"})
    print(data.get("message", "runner resumed"))
    return 0
