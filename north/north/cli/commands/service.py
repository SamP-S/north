"""Aggregate service command: OS/process status for both systemd units."""

import argparse

from north.cli.commands import lifecycle
from north.cli.context import NorthContext


def aggregate_status(_args: argparse.Namespace, _ctx: NorthContext) -> int:
    """Print OS/process status for both the aurora and borealis units."""
    for unit in ("aurora", "borealis"):
        for line in lifecycle._status_lines(unit):
            print(line)
    return 0
