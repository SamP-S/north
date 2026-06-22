"""Tests for the logs command (SSE stream)."""

import httpx
import pytest
from north.cli.commands import observe

_SSE = (
    'data: {"type": "agent.output", "project": "p", "feature": "f", '
    '"task_id": "t1", "output": "hello"}\n'
    "\n"
    'data: {"type": "heartbeat"}\n'
    "\n"
    'data: {"type": "agent.output", "project": "other", "feature": "g", '
    '"task_id": "t2", "output": "world"}\n'
)


def test_logs_prints_agent_output(
    ns, make_ctx, capsys: pytest.CaptureFixture[str]
) -> None:
    def handler(_request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, text=_SSE)

    rc = observe.logs(ns(project=None), make_ctx(aurora=handler))
    out = capsys.readouterr().out
    assert rc == 0
    assert "[p/f/t1] hello" in out
    assert "[other/g/t2] world" in out
    # heartbeat (non agent.output) is skipped
    assert "heartbeat" not in out


def test_logs_filters_by_project(
    ns, make_ctx, capsys: pytest.CaptureFixture[str]
) -> None:
    def handler(_request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, text=_SSE)

    rc = observe.logs(ns(project="p"), make_ctx(aurora=handler))
    out = capsys.readouterr().out
    assert rc == 0
    assert "[p/f/t1] hello" in out
    assert "world" not in out


_QUEUE = [
    {"task_id": "t1", "status": "in_progress", "project": "p", "feature": "f", "title": "A"},
    {"task_id": "t2", "status": "queued", "project": "p", "feature": "f", "title": "B"},
]


def test_queue_lists_tasks(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    rc = observe.queue(
        ns(project=None), make_ctx(borealis=lambda r: httpx.Response(200, json=_QUEUE))
    )
    out = capsys.readouterr().out
    assert rc == 0
    assert "t1  [in_progress]  p/f  A" in out
    assert "t2  [queued]  p/f  B" in out


def test_queue_empty(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    rc = observe.queue(
        ns(project=None), make_ctx(borealis=lambda r: httpx.Response(200, json=[]))
    )
    assert rc == 0
    assert "queue is empty" in capsys.readouterr().out


def test_queue_passes_project_filter(ns, make_ctx) -> None:
    seen: dict[str, object] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        seen["query"] = request.url.params.get("project")
        return httpx.Response(200, json=[])

    observe.queue(ns(project="demo"), make_ctx(borealis=handler))
    assert seen["query"] == "demo"
