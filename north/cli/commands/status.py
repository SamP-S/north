"""Status command: the North board health block."""

import argparse
from typing import Any

from north.cli.context import NorthContext


def status(_args: argparse.Namespace, ctx: NorthContext) -> int:
    """Print the North board-status block; raise CLIError on fetch failure."""
    data: dict[str, Any] = ctx.board.get("/api/status")
    print(f"runner state:  {data.get('runner_state', 'unknown')}")
    print(f"board loaded:  {data.get('board_loaded', 'unknown')}")
    return 0
