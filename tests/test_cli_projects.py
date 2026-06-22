"""Tests for the projects commands: list, show, register, unregister."""

import json

import httpx
import pytest

from north.cli.clients.errors import CLIError
from north.cli.commands import projects


def test_list_empty(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    def handler(_request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json=[])

    rc = projects.list_(ns(), make_ctx(board=handler))
    assert rc == 0
    assert "no projects registered" in capsys.readouterr().out


def test_list_renders_projects(
    ns, make_ctx, capsys: pytest.CaptureFixture[str]
) -> None:
    def handler(_request: httpx.Request) -> httpx.Response:
        return httpx.Response(
            200,
            json=[{"name": "app", "ssh_url": "git@h:me/app.git", "base_branch": "main"}],
        )

    rc = projects.list_(ns(), make_ctx(board=handler))
    out = capsys.readouterr().out
    assert rc == 0
    assert "app" in out
    assert "git@h:me/app.git" in out
    assert "(main)" in out


def test_show_renders_fields(
    ns, make_ctx, capsys: pytest.CaptureFixture[str]
) -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/api/projects/app"
        return httpx.Response(
            200,
            json={
                "name": "app",
                "ssh_url": "git@h:me/app.git",
                "base_branch": "main",
                "auto_merge": True,
            },
        )

    rc = projects.show(ns(project="app"), make_ctx(board=handler))
    out = capsys.readouterr().out
    assert rc == 0
    assert "name:" in out and "app" in out
    assert "auto_merge:" in out and "True" in out


def test_register_posts_body(
    ns, make_ctx, capsys: pytest.CaptureFixture[str]
) -> None:
    seen: dict[str, object] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        seen.update(json.loads(request.content))
        return httpx.Response(200, json={"name": "app"})

    rc = projects.register(
        ns(ssh_url="git@h:me/app.git", name=None, base_branch="main", auto_merge=True),
        make_ctx(board=handler),
    )
    out = capsys.readouterr().out
    assert rc == 0
    assert seen == {"ssh_url": "git@h:me/app.git", "base_branch": "main", "auto_merge": True}
    assert "registered project: app" in out


def test_unregister_declined_skips_delete(
    ns, make_ctx, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    calls: list[str] = []

    def handler(request: httpx.Request) -> httpx.Response:
        calls.append(request.method)
        if request.method == "GET":
            return httpx.Response(200, json=[])
        return httpx.Response(200, json={})

    monkeypatch.setattr("builtins.input", lambda _prompt: "n")
    rc = projects.unregister(ns(project="app", yes=False), make_ctx(board=handler))
    out = capsys.readouterr().out
    assert rc == 0
    assert "DELETE" not in calls
    assert "aborted" in out


def test_unregister_confirmed_deletes(
    ns, make_ctx, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    calls: list[str] = []

    def handler(request: httpx.Request) -> httpx.Response:
        calls.append(request.method)
        if request.method == "GET":
            return httpx.Response(200, json=[])
        return httpx.Response(200, json={})

    monkeypatch.setattr("builtins.input", lambda _prompt: "y")
    rc = projects.unregister(ns(project="app", yes=False), make_ctx(board=handler))
    out = capsys.readouterr().out
    assert rc == 0
    assert "DELETE" in calls
    assert "unregistered project: app" in out


def test_update_base_branch(ns, make_ctx, capsys: pytest.CaptureFixture[str]) -> None:
    seen: dict[str, object] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        seen["method"] = request.method
        seen["path"] = request.url.path
        seen["body"] = json.loads(request.content)
        return httpx.Response(200, json={"base_branch": "develop", "auto_merge": False})

    rc = projects.update(
        ns(project="app", base_branch="develop", auto_merge=None),
        make_ctx(board=handler),
    )
    out = capsys.readouterr().out
    assert rc == 0
    assert seen["method"] == "PATCH"
    assert seen["path"] == "/api/projects/app"
    assert seen["body"] == {"base_branch": "develop"}
    assert "updated project: app" in out
    assert "base_branch:" in out and "develop" in out


def test_update_auto_merge_flag(ns, make_ctx) -> None:
    seen: dict[str, object] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        seen["body"] = json.loads(request.content)
        return httpx.Response(200, json={"base_branch": "main", "auto_merge": True})

    rc = projects.update(
        ns(project="app", base_branch=None, auto_merge=True), make_ctx(board=handler)
    )
    assert rc == 0
    assert seen["body"] == {"auto_merge": True}


def test_update_no_fields_raises(ns, make_ctx) -> None:
    with pytest.raises(CLIError, match="nothing to update"):
        projects.update(
            ns(project="app", base_branch=None, auto_merge=None),
            make_ctx(board=lambda r: httpx.Response(200, json={})),
        )


def test_unregister_yes_skips_prompt(ns, make_ctx) -> None:
    calls: list[str] = []

    def handler(request: httpx.Request) -> httpx.Response:
        calls.append(request.method)
        if request.method == "GET":
            return httpx.Response(200, json=[])
        return httpx.Response(200, json={})

    rc = projects.unregister(ns(project="app", yes=True), make_ctx(board=handler))
    assert rc == 0
    assert "DELETE" in calls
