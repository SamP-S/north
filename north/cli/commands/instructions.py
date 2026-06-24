"""`north instructions` — print the agent guidance (same text as AGENTS.md)."""

import argparse

from north.core.instructions import agents_md


def instructions(args: argparse.Namespace) -> int:
    print(agents_md())
    return 0
