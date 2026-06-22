"""Tests for the North MCP surface: grant filtering, token auth, write parity."""

import subprocess
from contextlib import asynccontextmanager
from pathlib import Path

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from north.service.api.deps import set_board_context
from north.service.mcp import GRANTS, mount_mcp, parse_token_map
from north.service.models import BoardState, ProjectModel

_HEADERS = {
    "Accept": "application/json, text/event-stream",
    "Content-Type": "application/json",
}


def _run(cwd: Path, *args: str) -> None:
    subprocess.run(["git", *args], cwd=str(cwd), check=True, capture_output=True)


@pytest.fixture
def board_repo(tmp_path: Path) -> Path:
    repo = tmp_path / "board"
    repo.mkdir()
    _run(repo, "init", "-b", "main")
    _run(repo, "config", "user.email", "test@test.com")
    _run(repo, "config", "user.name", "Test")
    (repo / ".keep").write_text("")
    _run(repo, "add", ".keep")
    _run(repo, "commit", "-m", "init")
    return repo


def _make_client(
    board_repo: Path, token_map: dict[str, str] | None = None
) -> tuple[TestClient, BoardState]:
    state = BoardState()
    state.projects["demo"] = ProjectModel(name="demo", ssh_url="git@example.com:demo.git")
    set_board_context(lambda: state, board_repo)

    app = FastAPI()
    session_managers = mount_mcp(
        app, token_map=token_map, allowed_hosts=["testserver", "127.0.0.1:*"]
    )

    @asynccontextmanager
    async def lifespan(app: FastAPI):
        async with session_managers():
            yield

    app.router.lifespan_context = lifespan
    return TestClient(app), state


def _rpc(client: TestClient, grant: str, method: str, params: dict | None = None,
         token: str | None = None) -> dict:
    headers = dict(_HEADERS)
    if token:
        headers["Authorization"] = f"Bearer {token}"
    payload: dict = {"jsonrpc": "2.0", "id": 1, "method": method}
    if params is not None:
        payload["params"] = params
    resp = client.post(f"/mcp/{grant}", json=payload, headers=headers)
    if resp.status_code != 200:
        return {"_status": resp.status_code}
    return resp.json()


def _tool_names(client: TestClient, grant: str, token: str | None = None) -> set[str]:
    data = _rpc(client, grant, "tools/list", token=token)
    return {t["name"] for t in data["result"]["tools"]}


def _call(client: TestClient, grant: str, name: str, arguments: dict,
          token: str | None = None) -> dict:
    return _rpc(
        client, grant, "tools/call", {"name": name, "arguments": arguments}, token=token
    )


def _board_log(board_repo: Path) -> list[str]:
    out = subprocess.run(
        ["git", "log", "--format=%s"],
        cwd=str(board_repo),
        check=True,
        capture_output=True,
        text=True,
    )
    return out.stdout.strip().splitlines()


def test_tool_list_matches_grant_sets(board_repo: Path) -> None:
    client, _ = _make_client(board_repo)
    with client:
        for grant, expected in GRANTS.items():
            assert _tool_names(client, grant) == expected, grant


def test_write_through_mcp_matches_rest_commit(board_repo: Path) -> None:
    client, state = _make_client(board_repo)
    with client:
        result = _call(
            client,
            "cockpit",
            "create_conversation",
            {"project": "demo", "title": "Adaptive feeds", "content": "feeds talk"},
        )
        assert result["result"]["isError"] is False
    # identical board commit to the REST verb
    assert _board_log(board_repo)[0] == "[board:conversation] create demo/001"
    assert "001" in state.projects["demo"].conversations


def test_grant_without_tool_cannot_call_it(board_repo: Path) -> None:
    client, _ = _make_client(board_repo)
    with client:
        result = _call(
            client,
            "reviewer",
            "create_task",
            {"project": "demo", "feature": "f", "title": "t", "pipeline": "p"},
        )
        # unknown tool for this grant: protocol-level error, no board write
        assert (
            result.get("error") is not None
            or result["result"].get("isError") is True
        )
    assert "[board:task]" not in "".join(_board_log(board_repo))


def test_api_errors_surface_as_tool_errors(board_repo: Path) -> None:
    client, _ = _make_client(board_repo)
    with client:
        result = _call(
            client, "cockpit", "get_conversation",
            {"project": "demo", "conversation_id": "999"},
        )
        assert result["result"]["isError"] is True
        assert "404" in result["result"]["content"][0]["text"]


def test_token_guard(board_repo: Path) -> None:
    client, _ = _make_client(board_repo, token_map={"cockpit": "sekrit"})
    with client:
        assert _rpc(client, "cockpit", "tools/list")["_status"] == 401
        assert _rpc(client, "cockpit", "tools/list", token="wrong")["_status"] == 401
        assert "get_queue" in _tool_names(client, "cockpit", token="sekrit")
        # grants without configured tokens stay open (loopback-only service)
        assert "get_queue" in _tool_names(client, "decomposer")


def test_parse_token_map() -> None:
    assert parse_token_map("") == {}
    assert parse_token_map("cockpit:abc, decomposer:def") == {
        "cockpit": "abc",
        "decomposer": "def",
    }
    with pytest.raises(ValueError):
        parse_token_map("nope:abc")
    with pytest.raises(ValueError):
        parse_token_map("cockpit")
