"""Conversation commands: create (ship), list, show, status."""

import argparse
from pathlib import Path
from typing import Any

from north.cli.clients.errors import CLIError
from north.cli.context import NorthContext


def create(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Ship a condensed conversation onto the board."""
    if args.content and args.content_file:
        raise CLIError("--content and --content-file are mutually exclusive")
    content = args.content or ""
    if args.content_file:
        path = Path(args.content_file)
        if not path.is_file():
            raise CLIError(f"content file not found: {args.content_file}")
        content = path.read_text(encoding="utf-8")

    data: dict[str, Any] = ctx.board.post(
        f"/api/projects/{args.project}/conversations",
        body={"title": args.title, "content": content, "source": args.source},
    )
    print(f"created conversation: {args.project}/{data.get('conversation_id')}")
    return 0


def list_(args: argparse.Namespace, ctx: NorthContext) -> int:
    """List conversations: a project's, or the cross-project pending queue."""
    if getattr(args, "pending", False):
        params = {"project": args.project} if args.project else None
        data: list[dict[str, Any]] = ctx.board.get(
            "/api/conversations/pending", params=params
        )
        if not data:
            print("no pending conversations")
            return 0
        for conv in data:
            print(
                f"{conv.get('conversation_id'):>4}  {conv.get('status'):12} "
                f"{conv.get('project')}  {conv.get('title')}"
            )
        return 0

    if not args.project:
        raise CLIError("project is required (or pass --pending)")
    data = ctx.board.get(f"/api/projects/{args.project}/conversations")
    if not data:
        print("no conversations")
        return 0
    for conv in data:
        print(
            f"{conv.get('conversation_id'):>4}  {conv.get('status'):12} "
            f"{conv.get('source'):5}  {conv.get('title')}"
        )
    return 0


def show(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Show a conversation, including its result content if present."""
    data: dict[str, Any] = ctx.board.get(
        f"/api/projects/{args.project}/conversations/{args.conversation_id}"
    )
    for key in ("conversation_id", "title", "status", "source", "created_at", "decomposed_into"):
        print(f"{key + ':':17} {data.get(key)}")
    body = data.get("body")
    if body:
        print(f"\n{body}")
    result_content = data.get("result_content")
    if result_content:
        print(f"\n--- result ---\n{result_content}")
    return 0


def status(args: argparse.Namespace, ctx: NorthContext) -> int:
    """Set a conversation's status."""
    data: dict[str, Any] = ctx.board.patch(
        f"/api/projects/{args.project}/conversations/{args.conversation_id}/status",
        body={"status": args.status},
    )
    print(f"{args.project}/{args.conversation_id} → {data.get('status', args.status)}")
    return 0
