"""`north board` — a quick board summary (counts per status)."""

import argparse

from north.core.board import locate_board
from north.core.tasks import status_counts


def board(args: argparse.Namespace) -> int:
    counts = status_counts(locate_board())
    width = max(len(name) for name in counts)
    for name, count in counts.items():
        print(f"{name:<{width}}  {count}")
    print(f"{'total':<{width}}  {sum(counts.values())}")
    return 0
