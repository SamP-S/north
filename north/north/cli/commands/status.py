"""Status commands: combined view plus per-service blocks."""

import argparse
from typing import Any

from north.cli.clients.errors import CLIError
from north.cli.context import NorthContext


def _aurora_block(ctx: NorthContext) -> None:
    """Print the Aurora runner-status block; raise CLIError on fetch failure."""
    data: dict[str, Any] = ctx.aurora.get("/api/status")
    print(f"runner state:   {data.get('runner_state', 'unknown')}")
    print(f"active project: {data.get('active_project') or '-'}")
    print(f"active task:    {data.get('active_task') or '-'}")
    print(f"oauth health:   {data.get('oauth_health', 'unknown')}")


def _borealis_block(ctx: NorthContext) -> None:
    """Print the Borealis board-status block; raise CLIError on fetch failure."""
    data: dict[str, Any] = ctx.borealis.get("/api/status")
    print(f"runner state:  {data.get('runner_state', 'unknown')}")
    print(f"board loaded:  {data.get('board_loaded', 'unknown')}")


def aurora(_args: argparse.Namespace, ctx: NorthContext) -> int:
    """Print the Aurora runner-status block."""
    _aurora_block(ctx)
    return 0


def borealis(_args: argparse.Namespace, ctx: NorthContext) -> int:
    """Print the Borealis board-status block."""
    _borealis_block(ctx)
    return 0


def status(_args: argparse.Namespace, ctx: NorthContext) -> int:
    """Print the combined Aurora and Borealis status.

    Each block is fetched independently: a CLIError in one prints an error line
    under its heading without aborting the other. Returns non-zero if either
    block failed.
    """
    failed = False

    print("aurora")
    try:
        _aurora_block(ctx)
    except CLIError as exc:
        print(f"  error: {exc}")
        failed = True

    print("borealis")
    try:
        _borealis_block(ctx)
    except CLIError as exc:
        print(f"  error: {exc}")
        failed = True

    return 1 if failed else 0
