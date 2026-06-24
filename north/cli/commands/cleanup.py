"""`north cleanup` — bulk-archive done tasks to keep the board readable."""

import argparse

from north.core.board import locate_board
from north.core.tasks import cleanup as core_cleanup


def cleanup(args: argparse.Namespace) -> int:
    archived = core_cleanup(locate_board(), older_than_days=args.older_than)
    if not archived:
        print("Nothing to clean up.")
        return 0
    print(f"Archived {len(archived)} done task(s): {', '.join(t.id for t in archived)}")
    return 0
