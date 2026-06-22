"""Tests for the feature review commands: approve, rollback, reject."""

import httpx
import pytest
from north.cli.commands import review


def test_approve_success_prints_merged(
    ns, make_ctx, capsys: pytest.CaptureFixture[str]
) -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/api/features/p/f/approve"
        return httpx.Response(200, json={"feature_status": "merged"})

    rc = review.approve(ns(project="p", feature="f"), make_ctx(aurora=handler))
    out = capsys.readouterr().out
    assert rc == 0
    assert "feature merged" in out


def test_approve_409_conflict_nonzero(
    ns, make_ctx, capsys: pytest.CaptureFixture[str]
) -> None:
    def handler(_request: httpx.Request) -> httpx.Response:
        return httpx.Response(409, json={"detail": "CONFLICT in file.py"})

    rc = review.approve(ns(project="p", feature="f"), make_ctx(aurora=handler))
    out = capsys.readouterr().out
    assert rc == 1
    assert "conflict" in out.lower()
    assert "409" in out


def test_rollback_prints_commit_warning(
    ns, make_ctx, capsys: pytest.CaptureFixture[str]
) -> None:
    def handler(_request: httpx.Request) -> httpx.Response:
        return httpx.Response(
            200,
            json={
                "reverted_commits": 2,
                "summaries": ["abc fix one", "def fix two"],
                "warning": "Rolling back 2 commit(s); tasks re-queued to ready",
            },
        )

    rc = review.rollback(ns(project="p", feature="f"), make_ctx(aurora=handler))
    out = capsys.readouterr().out
    assert rc == 0
    assert "Rolling back 2 commit(s)" in out
    assert "abc fix one" in out
    assert "rolled back" in out


def test_reject_declined_aborts(
    ns, make_ctx, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    posted = {"hit": False}

    def handler(_request: httpx.Request) -> httpx.Response:
        posted["hit"] = True
        return httpx.Response(200, json={})

    monkeypatch.setattr("builtins.input", lambda _prompt: "n")
    rc = review.reject(ns(project="p", feature="f", yes=False), make_ctx(aurora=handler))
    out = capsys.readouterr().out
    assert rc == 0
    assert posted["hit"] is False
    assert "aborted" in out


def test_reject_confirmed_executes(
    ns, make_ctx, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    posted = {"hit": False}

    def handler(_request: httpx.Request) -> httpx.Response:
        posted["hit"] = True
        return httpx.Response(200, json={})

    monkeypatch.setattr("builtins.input", lambda _prompt: "y")
    rc = review.reject(ns(project="p", feature="f", yes=False), make_ctx(aurora=handler))
    assert rc == 0
    assert posted["hit"] is True
    assert "rejected" in capsys.readouterr().out
