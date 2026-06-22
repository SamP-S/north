"""Tests for the queue command."""

import httpx
import pytest

from north.cli.commands import observe

_QUEUE = [
    {"task_id": "t1", "status": "in_progress", "project": "p", "feature": "f", "title": "A"},
    {"task_id": "t2", "status": "queued", "project": "p", "feature": "f", "title": "B"},
]


def test_queue_lists_tasks(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    rc = observe.queue(
        ns(project=None), make_ctx(board=lambda r: httpx.Response(200, json=_QUEUE))
    )
    out = capsys.readouterr().out
    assert rc == 0
    assert "t1  [in_progress]  p/f  A" in out
    assert "t2  [queued]  p/f  B" in out


def test_queue_empty(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    rc = observe.queue(
        ns(project=None), make_ctx(board=lambda r: httpx.Response(200, json=[]))
    )
    assert rc == 0
    assert "queue is empty" in capsys.readouterr().out


def test_queue_passes_project_filter(ns, make_ctx) -> None:
    seen: dict[str, object] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        seen["query"] = request.url.params.get("project")
        return httpx.Response(200, json=[])

    observe.queue(ns(project="demo"), make_ctx(board=handler))
    assert seen["query"] == "demo"
