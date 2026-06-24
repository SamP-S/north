"""`north init` — scaffold a board in the current repo."""

import argparse
from pathlib import Path

from north.core.board import init_board


def init(args: argparse.Namespace) -> int:
    """Create ``north/`` (config + status folders + archive) and ``AGENTS.md``."""
    board = init_board(Path.cwd())
    print(f"Initialized north board at {board}")
    return 0
