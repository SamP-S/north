"""Project management commands: list, show, register, unregister."""

import argparse
from typing import Any

from north.cli.clients.errors import CLIError
from north.cli.context import NorthContext
from north.cli.prompts import confirm


def list_(_args: argparse.Namespace, ctx: NorthContext) -> int:
    """List registered projects with their SSH URL and base branch."""
    items: list[dict[str, Any]] = ctx.board.get("/api/projects")
    if not items:
        print("no projects registered")
        return 0
    for project in items:
        print(
            f"{project.get('name')}  {project.get('ssh_url')}  "
            f"({project.get('base_branch')})"
        )
    return 0


def show(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Show a single project's name, SSH URL, base branch and auto-merge flag."""
    data: dict[str, Any] = ctx.board.get(f"/api/projects/{args.project}")
    for key in ("name", "ssh_url", "base_branch", "auto_merge"):
        print(f"{key + ':':13} {data.get(key)}")
    return 0


def register(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Register a new project from an SSH URL."""
    body: dict[str, Any] = {
        "ssh_url": args.ssh_url,
        "base_branch": args.base_branch,
        "auto_merge": args.auto_merge,
    }
    if args.name:
        body["name"] = args.name
    data: dict[str, Any] = ctx.board.post("/api/projects/register", body=body)
    name = (
        data.get("name")
        or args.name
        or args.ssh_url.rstrip("/").rsplit("/", 1)[-1].removesuffix(".git")
    )
    print(f"registered project: {name}")
    return 0


def update(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Update a project's base branch and/or auto-merge flag."""
    body: dict[str, Any] = {}
    if args.base_branch is not None:
        body["base_branch"] = args.base_branch
    if args.auto_merge is not None:
        body["auto_merge"] = args.auto_merge
    if not body:
        raise CLIError(
            "nothing to update: pass --base-branch and/or --auto-merge/--no-auto-merge"
        )
    data: dict[str, Any] = ctx.board.patch(f"/api/projects/{args.project}", body=body)
    print(f"updated project: {args.project}")
    for key in ("base_branch", "auto_merge"):
        print(f"{key + ':':13} {data.get(key)}")
    return 0


def unregister(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Unregister a project after warning about its active features."""
    project = args.project
    active: list[Any] = []
    try:
        features: list[dict[str, Any]] = ctx.board.get(
            "/api/features", params={"project": project}
        )
        active = [
            f
            for f in features
            if f.get("status") in ("active", "in_progress", "open", "review")
        ]
    except Exception:  # noqa: BLE001 — warning is best-effort
        active = []

    if active:
        print(f"warning: {project} has {len(active)} active/in-progress feature(s):")
        for feature in active:
            print(f"  - {feature.get('feature_id')} [{feature.get('status')}]")

    if not args.yes and not confirm(f"Unregister {project}?"):
        print("aborted")
        return 0

    ctx.board.delete(f"/api/projects/{project}")
    print(f"unregistered project: {project}")
    return 0
