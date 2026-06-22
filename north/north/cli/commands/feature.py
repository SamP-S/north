"""Feature commands: create, show, status, delete, requeue, promote, list."""

import argparse
from typing import Any

from north.cli.clients.errors import CLIError
from north.cli.context import NorthContext
from north.cli.prompts import confirm


def create(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Create a new feature on a project's board."""
    body: dict[str, Any] = {
        "title": args.title,
        "description": args.description or "",
        "depends_on": args.depends_on or [],
    }
    data: dict[str, Any] = ctx.borealis.post(
        f"/api/projects/{args.project}/features", body=body
    )
    print(f"created feature: {args.project}/{data.get('feature_id')}")
    return 0


def show(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Show a single feature's details."""
    data: dict[str, Any] = ctx.borealis.get(
        f"/api/projects/{args.project}/features/{args.feature}"
    )
    for key in (
        "feature_id",
        "title",
        "status",
        "project",
        "branch",
        "depends_on",
        "created_at",
        "merged_at",
    ):
        print(f"{key + ':':13} {data.get(key)}")
    description = data.get("description")
    if description:
        print(f"\n{description}")
    return 0


def edit(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Edit a feature's fields; unspecified flags keep their current values."""
    current: dict[str, Any] = ctx.borealis.get(
        f"/api/projects/{args.project}/features/{args.feature}"
    )
    body: dict[str, Any] = {
        "title": args.title if args.title is not None else current.get("title"),
        "description": (
            args.description
            if args.description is not None
            else (current.get("description") or "")
        ),
        "depends_on": (
            args.depends_on
            if args.depends_on is not None
            else (current.get("depends_on") or [])
        ),
        "status": args.status if args.status is not None else current.get("status"),
    }
    ctx.borealis.put(f"/api/projects/{args.project}/features/{args.feature}", body=body)
    print(f"updated feature: {args.project}/{args.feature}")
    return 0


def status(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Set a feature's status."""
    data: dict[str, Any] = ctx.borealis.patch(
        f"/api/projects/{args.project}/features/{args.feature}/status",
        body={"status": args.status},
    )
    print(f"{args.project}/{args.feature} → {data.get('status', args.status)}")
    return 0


def delete(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Delete a feature directory entirely (draft tasks only)."""
    if not args.yes and not confirm(
        f"Delete feature {args.project}/{args.feature}? This removes its files"
    ):
        print("aborted")
        return 0
    ctx.borealis.delete(f"/api/projects/{args.project}/features/{args.feature}")
    print(f"deleted feature: {args.project}/{args.feature}")
    return 0


def requeue(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Re-open a feature and reset its tasks to ready."""
    data: dict[str, Any] = ctx.borealis.post(
        f"/api/projects/{args.project}/features/{args.feature}/requeue"
    )
    print(
        f"requeued {args.project}/{args.feature} "
        f"({data.get('requeued', 0)} task(s) reset to ready)"
    )
    return 0


def promote(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Promote a draft feature to open."""
    ctx.borealis.post(f"/api/projects/{args.project}/features/{args.feature}/promote")
    print(f"promoted feature: {args.project}/{args.feature} → open")
    return 0


def list_(args: argparse.Namespace, ctx: NorthContext) -> int:
    """List features: global, project-scoped, archived, or awaiting review."""
    client = ctx.borealis
    if args.review and args.archived:
        raise CLIError("--review and --archived are mutually exclusive")
    if args.review:
        items: list[dict[str, Any]] = client.get(
            "/api/review",
            params={"project": args.project} if args.project else None,
        )
        _print_features(items, "no features awaiting review")
        return 0
    if args.archived and not args.project:
        raise CLIError("--archived requires --project")
    if args.project:
        params = {"include": "archived"} if args.archived else None
        items = client.get(f"/api/projects/{args.project}/features", params=params)
    else:
        items = client.get("/api/features")
    _print_features(items, "no features")
    return 0


def _print_features(items: list[dict[str, Any]], empty_message: str) -> None:
    if not items:
        print(empty_message)
        return
    for feature in items:
        print(
            f"{feature.get('project')}/{feature.get('feature_id')}  "
            f"[{feature.get('status')}]  {feature.get('title')}  "
            f"({feature.get('branch')})"
        )
