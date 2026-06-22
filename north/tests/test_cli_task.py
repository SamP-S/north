"""Tests for the task commands, including the client-side --status filter."""

import json

import httpx
import pytest
from north.cli.clients.errors import CLIError
from north.cli.commands import task


def test_create_posts_body(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    seen: dict[str, object] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        seen.update(json.loads(request.content))
        return httpx.Response(200, json={"task_id": "t-1"})

    rc = task.create(
        ns(
            project="p",
            feature="f",
            title="do",
            pipeline="default",
            body="b",
            body_file=None,
            depends_on=None,
        ),
        make_ctx(borealis=handler),
    )
    assert rc == 0
    assert seen["pipeline"] == "default"
    assert seen["title"] == "do"
    assert "created task: p/f/t-1" in capsys.readouterr().out


def test_show_renders(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    def handler(_request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={"task_id": "t-1", "status": "ready"})

    rc = task.show(
        ns(project="p", feature="f", task_id="t-1"), make_ctx(borealis=handler)
    )
    out = capsys.readouterr().out
    assert rc == 0
    assert "task_id:" in out and "t-1" in out


_TASKS = [
    {"task_id": "t-1", "status": "done", "title": "A"},
    {"task_id": "t-2", "status": "ready", "title": "B"},
    {"task_id": "t-3", "status": "done", "title": "C"},
]


def test_list_no_filter(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    rc = task.list_(
        ns(project="p", feature="f", status=None),
        make_ctx(borealis=lambda r: httpx.Response(200, json=_TASKS)),
    )
    out = capsys.readouterr().out
    assert rc == 0
    assert "t-1" in out and "t-2" in out and "t-3" in out


def test_list_status_filter(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    rc = task.list_(
        ns(project="p", feature="f", status="done"),
        make_ctx(borealis=lambda r: httpx.Response(200, json=_TASKS)),
    )
    out = capsys.readouterr().out
    assert rc == 0
    assert "t-1" in out and "t-3" in out
    assert "t-2" not in out


def test_status_sets(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    def handler(_request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={"status": "in_progress"})

    rc = task.status(
        ns(project="p", feature="f", task_id="t-1", status="in_progress"),
        make_ctx(borealis=handler),
    )
    assert rc == 0
    assert "p/f/t-1 → in_progress" in capsys.readouterr().out


def test_delete_declined(
    ns, make_ctx, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    calls: list[str] = []
    monkeypatch.setattr("builtins.input", lambda _prompt: "n")
    rc = task.delete(
        ns(project="p", feature="f", task_id="t-1", yes=False),
        make_ctx(borealis=lambda r: calls.append(r.method) or httpx.Response(200, json={})),
    )
    assert rc == 0
    assert "DELETE" not in calls
    assert "aborted" in capsys.readouterr().out


def test_delete_confirmed(
    ns, make_ctx, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    calls: list[str] = []
    monkeypatch.setattr("builtins.input", lambda _prompt: "y")
    rc = task.delete(
        ns(project="p", feature="f", task_id="t-1", yes=False),
        make_ctx(borealis=lambda r: calls.append(r.method) or httpx.Response(200, json={})),
    )
    assert rc == 0
    assert "DELETE" in calls


def test_promote(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    rc = task.promote(
        ns(project="p", feature="f", task_id="t-1"),
        make_ctx(borealis=lambda r: httpx.Response(200, json={})),
    )
    assert rc == 0
    assert "promoted task: p/f/t-1 → ready" in capsys.readouterr().out


def test_split_posts_tasks(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    seen: dict[str, object] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        seen.update(json.loads(request.content))
        return httpx.Response(200, json={"created": ["t-2", "t-3"]})

    rc = task.split(
        ns(
            project="p",
            feature="f",
            task_id="t-1",
            tasks_json='[{"title": "A"}, {"title": "B"}]',
            tasks_file=None,
        ),
        make_ctx(borealis=handler),
    )
    out = capsys.readouterr().out
    assert rc == 0
    assert seen["tasks"] == [{"title": "A"}, {"title": "B"}]
    assert "t-2, t-3" in out


def test_edit_merges_current(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    current = {
        "title": "Old",
        "pipeline": "default",
        "body": "old body",
        "depends_on": [],
        "status": "ready",
    }
    seen: dict[str, object] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        if request.method == "GET":
            return httpx.Response(200, json=current)
        seen.update(json.loads(request.content))
        return httpx.Response(200, json={"message": "ok"})

    rc = task.edit(
        ns(
            project="p",
            feature="f",
            task_id="t-1",
            title=None,
            pipeline="local",
            body=None,
            body_file=None,
            depends_on=None,
            status=None,
        ),
        make_ctx(borealis=handler),
    )
    assert rc == 0
    assert seen == {
        "title": "Old",
        "pipeline": "local",
        "body": "old body",
        "depends_on": [],
        "status": "ready",
    }
    assert "updated task: p/f/t-1" in capsys.readouterr().out


def test_edit_body_exclusivity(ns, make_ctx) -> None:
    with pytest.raises(CLIError, match="mutually exclusive"):
        task.edit(
            ns(
                project="p",
                feature="f",
                task_id="t-1",
                title=None,
                pipeline=None,
                body="x",
                body_file="y.md",
                depends_on=None,
                status=None,
            ),
            make_ctx(borealis=lambda r: httpx.Response(200, json={})),
        )
