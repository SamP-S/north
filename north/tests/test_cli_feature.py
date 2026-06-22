"""Tests for the feature commands, including the consolidated list_."""

import json

import httpx
import pytest
from north.cli.clients.errors import CLIError
from north.cli.commands import feature


def _dummy(_request: httpx.Request) -> httpx.Response:
    return httpx.Response(200, json={})


def test_create_prints_feature_id(
    ns, make_ctx, capsys: pytest.CaptureFixture[str]
) -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/api/projects/p/features"
        return httpx.Response(200, json={"feature_id": "feat-1"})

    rc = feature.create(
        ns(project="p", title="t", description=None, depends_on=None),
        make_ctx(borealis=handler),
    )
    assert rc == 0
    assert "created feature: p/feat-1" in capsys.readouterr().out


def test_show_renders(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    def handler(_request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={"feature_id": "feat-1", "status": "open"})

    rc = feature.show(ns(project="p", feature="feat-1"), make_ctx(borealis=handler))
    out = capsys.readouterr().out
    assert rc == 0
    assert "feature_id:" in out and "feat-1" in out


def test_status_sets(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    def handler(_request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={"status": "in_progress"})

    rc = feature.status(
        ns(project="p", feature="f", status="in_progress"), make_ctx(borealis=handler)
    )
    assert rc == 0
    assert "p/f → in_progress" in capsys.readouterr().out


def test_delete_declined(
    ns, make_ctx, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    calls: list[str] = []
    monkeypatch.setattr("builtins.input", lambda _prompt: "n")
    rc = feature.delete(
        ns(project="p", feature="f", yes=False),
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
    rc = feature.delete(
        ns(project="p", feature="f", yes=False),
        make_ctx(borealis=lambda r: calls.append(r.method) or httpx.Response(200, json={})),
    )
    assert rc == 0
    assert "DELETE" in calls
    assert "deleted feature: p/f" in capsys.readouterr().out


def test_requeue(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    rc = feature.requeue(
        ns(project="p", feature="f"),
        make_ctx(borealis=lambda r: httpx.Response(200, json={"requeued": 3})),
    )
    assert rc == 0
    assert "requeued p/f" in capsys.readouterr().out


def test_promote(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    rc = feature.promote(ns(project="p", feature="f"), make_ctx(borealis=_dummy))
    assert rc == 0
    assert "promoted feature: p/f → open" in capsys.readouterr().out


def test_list_review(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/api/review"
        return httpx.Response(
            200, json=[{"project": "p", "feature_id": "f", "status": "review", "title": "T"}]
        )

    rc = feature.list_(
        ns(review=True, archived=False, project=None), make_ctx(borealis=handler)
    )
    out = capsys.readouterr().out
    assert rc == 0
    assert "p/f" in out and "[review]" in out


def test_list_project_scoped(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/api/projects/p/features"
        return httpx.Response(200, json=[])

    rc = feature.list_(
        ns(review=False, archived=False, project="p"), make_ctx(borealis=handler)
    )
    assert rc == 0
    assert "no features" in capsys.readouterr().out


def test_list_all(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/api/features"
        return httpx.Response(200, json=[])

    rc = feature.list_(
        ns(review=False, archived=False, project=None), make_ctx(borealis=handler)
    )
    assert rc == 0


def test_list_archived_include(ns, make_ctx) -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        assert request.url.params.get("include") == "archived"
        return httpx.Response(200, json=[])

    rc = feature.list_(
        ns(review=False, archived=True, project="p"), make_ctx(borealis=handler)
    )
    assert rc == 0


def test_list_review_and_archived_mutually_exclusive(ns, make_ctx) -> None:
    with pytest.raises(CLIError, match="mutually exclusive"):
        feature.list_(
            ns(review=True, archived=True, project=None), make_ctx(borealis=_dummy)
        )


def test_list_archived_requires_project(ns, make_ctx) -> None:
    with pytest.raises(CLIError, match="requires --project"):
        feature.list_(
            ns(review=False, archived=True, project=None), make_ctx(borealis=_dummy)
        )


def test_edit_merges_current(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    current = {
        "title": "Old",
        "description": "old desc",
        "depends_on": ["feat-a"],
        "status": "open",
    }
    seen: dict[str, object] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        if request.method == "GET":
            return httpx.Response(200, json=current)
        seen.update(json.loads(request.content))
        return httpx.Response(200, json={"message": "ok"})

    rc = feature.edit(
        ns(
            project="p",
            feature="f",
            title="New",
            description=None,
            depends_on=None,
            status=None,
        ),
        make_ctx(borealis=handler),
    )
    assert rc == 0
    assert seen == {
        "title": "New",
        "description": "old desc",
        "depends_on": ["feat-a"],
        "status": "open",
    }
    assert "updated feature: p/f" in capsys.readouterr().out


def test_edit_clears_depends_on(ns, make_ctx) -> None:
    current = {"title": "T", "description": "", "depends_on": ["feat-a"], "status": "open"}
    seen: dict[str, object] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        if request.method == "GET":
            return httpx.Response(200, json=current)
        seen.update(json.loads(request.content))
        return httpx.Response(200, json={"message": "ok"})

    feature.edit(
        ns(project="p", feature="f", title=None, description=None, depends_on=[], status=None),
        make_ctx(borealis=handler),
    )
    assert seen["depends_on"] == []
