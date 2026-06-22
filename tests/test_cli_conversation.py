"""Tests for the conversation commands, including the new status command."""

import json

import httpx
import pytest

from north.cli.clients.errors import CLIError
from north.cli.commands import conversation


def test_create_ships(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/api/projects/p/conversations"
        return httpx.Response(200, json={"conversation_id": "c-1"})

    rc = conversation.create(
        ns(project="p", title="T", content="hi", content_file=None, source="text"),
        make_ctx(board=handler),
    )
    assert rc == 0
    assert "created conversation: p/c-1" in capsys.readouterr().out


def test_list_empty(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    rc = conversation.list_(
        ns(project="p"), make_ctx(board=lambda r: httpx.Response(200, json=[]))
    )
    assert rc == 0
    assert "no conversations" in capsys.readouterr().out


def test_list_pending_cross_project(
    ns, make_ctx, capsys: pytest.CaptureFixture[str]
) -> None:
    seen: dict[str, object] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        seen["path"] = request.url.path
        return httpx.Response(
            200,
            json=[{"conversation_id": "c-1", "status": "pending", "project": "p", "title": "T"}],
        )

    rc = conversation.list_(ns(project=None, pending=True), make_ctx(board=handler))
    out = capsys.readouterr().out
    assert rc == 0
    assert seen["path"] == "/api/conversations/pending"
    assert "c-1" in out and "pending" in out and "p" in out


def test_list_requires_project_without_pending(ns, make_ctx) -> None:
    with pytest.raises(CLIError, match="project is required"):
        conversation.list_(
            ns(project=None, pending=False),
            make_ctx(board=lambda r: httpx.Response(200, json=[])),
        )


def test_show_renders(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    def handler(_request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={"conversation_id": "c-1", "status": "shipped"})

    rc = conversation.show(
        ns(project="p", conversation_id="c-1"), make_ctx(board=handler)
    )
    out = capsys.readouterr().out
    assert rc == 0
    assert "conversation_id:" in out and "c-1" in out


def test_status_patches(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    seen: dict[str, object] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        seen["method"] = request.method
        seen["path"] = request.url.path
        seen["body"] = json.loads(request.content)
        return httpx.Response(200, json={"status": "decomposed"})

    rc = conversation.status(
        ns(project="p", conversation_id="c-1", status="decomposed"),
        make_ctx(board=handler),
    )
    out = capsys.readouterr().out
    assert rc == 0
    assert seen["method"] == "PATCH"
    assert seen["path"] == "/api/projects/p/conversations/c-1/status"
    assert seen["body"] == {"status": "decomposed"}
    assert "p/c-1 → decomposed" in out
