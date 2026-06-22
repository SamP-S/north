"""Tests for the comment commands: add and list."""

import json

import httpx
import pytest

from north.cli.commands import comment


def test_add_feature_thread(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    seen: dict[str, object] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        seen["path"] = request.url.path
        seen["body"] = json.loads(request.content)
        return httpx.Response(200, json={})

    rc = comment.add(
        ns(project="p", feature="f", task_id=None, kind="note", author="cli", text="hi"),
        make_ctx(board=handler),
    )
    assert rc == 0
    assert seen["path"] == "/api/projects/p/features/f/comments"
    assert seen["body"] == {"kind": "note", "author": "cli", "text": "hi"}
    assert "comment added" in capsys.readouterr().out


def test_add_task_thread(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    seen: dict[str, object] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        seen["path"] = request.url.path
        return httpx.Response(200, json={"task_status": "blocked"})

    rc = comment.add(
        ns(
            project="p",
            feature="f",
            task_id="t-1",
            kind="question",
            author="cli",
            text="why?",
        ),
        make_ctx(board=handler),
    )
    out = capsys.readouterr().out
    assert rc == 0
    assert seen["path"] == "/api/projects/p/features/f/tasks/t-1/comments"
    assert "task → blocked" in out


def test_list_empty(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    rc = comment.list_(
        ns(project="p", feature="f", task_id=None),
        make_ctx(board=lambda r: httpx.Response(200, json=[])),
    )
    assert rc == 0
    assert "no comments" in capsys.readouterr().out


def test_list_renders_entries(
    ns, make_ctx, capsys: pytest.CaptureFixture[str]
) -> None:
    def handler(_request: httpx.Request) -> httpx.Response:
        return httpx.Response(
            200,
            json=[{"kind": "note", "author": "sam", "at": "2026-06-15", "text": "hello"}],
        )

    rc = comment.list_(
        ns(project="p", feature="f", task_id=None), make_ctx(board=handler)
    )
    out = capsys.readouterr().out
    assert rc == 0
    assert "[note] sam" in out
    assert "hello" in out
