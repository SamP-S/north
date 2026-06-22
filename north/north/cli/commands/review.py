"""Feature review commands: approve, rollback, reject."""

import argparse
from typing import Any

from north.cli.clients.errors import CLIError
from north.cli.context import NorthContext
from north.cli.prompts import confirm


def approve(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Approve and merge a feature; surface merge conflicts as a failure."""
    try:
        ctx.aurora.post(f"/api/features/{args.project}/{args.feature}/approve")
    except CLIError as exc:
        message = str(exc)
        if message.startswith("HTTP 409"):
            print(f"merge conflict: {message}", flush=True)
            return 1
        raise
    print("feature merged")
    return 0


def rollback(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Roll back a merged feature after showing the commit warning."""
    data: dict[str, Any] = ctx.aurora.post(
        f"/api/features/{args.project}/{args.feature}/rollback"
    )
    warning = data.get("warning")
    if warning:
        print(warning)
    for summary in data.get("summaries", []):
        print(f"  - {summary}")
    print(f"feature {args.project}/{args.feature} rolled back")
    return 0


def reject(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Reject (discard) a feature branch after a confirmation prompt."""
    if not args.yes and not confirm(
        f"Reject {args.project}/{args.feature}? This discards the feature branch."
    ):
        print("aborted")
        return 0
    ctx.aurora.post(f"/api/features/{args.project}/{args.feature}/reject")
    print(f"feature {args.project}/{args.feature} rejected")
    return 0
