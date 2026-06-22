"""Tests for conversation board objects: parse, write round-trip, loader, REST."""

import subprocess
from datetime import UTC, datetime
from pathlib import Path

import pytest
from borealis.service.api.conversations import router as conversations_router
from borealis.service.api.deps import get_board_context
from borealis.service.board.loader import load_board_state
from borealis.service.board.parser import ParseError, parse_conversation, parse_task
from borealis.service.board.writer import (
    update_conversation_frontmatter,
    write_conversation_file,
)
from borealis.service.models import (
    BlockedReason,
    BoardState,
    ConversationStatus,
    ProjectModel,
)
from fastapi import FastAPI
from fastapi.testclient import TestClient


def _conversation_path(tmp_path: Path, content: str, name: str = "001.md") -> Path:
    p = tmp_path / "projects" / "demo" / "conversations" / name
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(content, encoding="utf-8")
    return p


# --- parse_conversation -------------------------------------------------------


def test_parse_conversation_minimal(tmp_path: Path) -> None:
    path = _conversation_path(
        tmp_path,
        "---\nid: 1\ntitle: Adaptive feeds\nstatus: pending\n---\n\nWe discussed feeds.\n",
    )
    conversation = parse_conversation(path)
    assert conversation.conversation_id == "001"
    assert conversation.title == "Adaptive feeds"
    assert conversation.status == ConversationStatus.PENDING
    assert conversation.source == "text"
    assert conversation.decomposed_into == []
    assert conversation.body == "We discussed feeds."


def test_parse_conversation_full(tmp_path: Path) -> None:
    path = _conversation_path(
        tmp_path,
        (
            "---\nid: 2\ntitle: T\nstatus: decomposed\nsource: voice\n"
            "created_at: '2026-06-12T10:00:00+00:00'\n"
            "decomposed_into:\n- feat-x\n- feat-x/001\n---\n\nBody.\n"
        ),
    )
    conversation = parse_conversation(path)
    assert conversation.status == ConversationStatus.DECOMPOSED
    assert conversation.source == "voice"
    assert conversation.created_at == datetime(2026, 6, 12, 10, 0, tzinfo=UTC)
    assert conversation.decomposed_into == ["feat-x", "feat-x/001"]


def test_parse_conversation_invalid_status_raises(tmp_path: Path) -> None:
    path = _conversation_path(
        tmp_path, "---\nid: 1\ntitle: T\nstatus: bogus\n---\n"
    )
    with pytest.raises(ParseError):
        parse_conversation(path)


# --- write round-trip ----------------------------------------------------------


def test_write_conversation_round_trip(tmp_path: Path) -> None:
    conversations_dir = tmp_path / "projects" / "demo" / "conversations"
    meta = {
        "id": "001",
        "title": "Adaptive feeds",
        "status": "pending",
        "source": "voice",
        "created_at": "2026-06-12T10:00:00+00:00",
    }
    path = write_conversation_file(conversations_dir, "001", meta, "We discussed feeds.")
    conversation = parse_conversation(path)
    assert conversation.conversation_id == "001"
    assert conversation.status == ConversationStatus.PENDING
    assert conversation.source == "voice"
    assert conversation.body == "We discussed feeds."


def test_update_conversation_frontmatter_preserves_body(tmp_path: Path) -> None:
    conversations_dir = tmp_path / "projects" / "demo" / "conversations"
    meta = {"id": "001", "title": "T", "status": "pending"}
    path = write_conversation_file(conversations_dir, "001", meta, "Body text.")
    update_conversation_frontmatter(
        path, {"status": "decomposed", "decomposed_into": ["feat-x"]}
    )
    conversation = parse_conversation(path)
    assert conversation.status == ConversationStatus.DECOMPOSED
    assert conversation.decomposed_into == ["feat-x"]
    assert conversation.body == "Body text."


# --- loader ---------------------------------------------------------------------


def _board_with_project(tmp_path: Path) -> Path:
    (tmp_path / "projects.yaml").write_text(
        "projects:\n  demo:\n    ssh_url: git@example.com:demo.git\n",
        encoding="utf-8",
    )
    (tmp_path / "projects" / "demo").mkdir(parents=True, exist_ok=True)
    return tmp_path


def test_loader_picks_up_conversations(tmp_path: Path) -> None:
    board = _board_with_project(tmp_path)
    _conversation_path(
        board, "---\nid: 1\ntitle: A\nstatus: pending\n---\nx\n", name="001.md"
    )
    _conversation_path(
        board, "---\nid: 2\ntitle: B\nstatus: decomposed\n---\ny\n", name="002.md"
    )
    # companions must be ignored
    _conversation_path(board, "handoff", name="001.result.md")
    state = load_board_state(board)
    conversations = state.projects["demo"].conversations
    assert sorted(conversations) == ["001", "002"]


def test_loader_skips_malformed_conversation(tmp_path: Path) -> None:
    board = _board_with_project(tmp_path)
    _conversation_path(board, "---\nid: 1\ntitle: A\nstatus: nope\n---\n", name="001.md")
    state = load_board_state(board)
    assert state.projects["demo"].conversations == {}


# --- task frontmatter extensions -------------------------------------------------


def test_parse_task_blocked_reason_and_split_from(tmp_path: Path) -> None:
    p = tmp_path / "001.md"
    p.write_text(
        (
            "---\nid: 1\ntitle: T\nstatus: blocked\npipeline: default\n"
            "blocked_reason: question\nsplit_from: 7\n---\n"
        ),
        encoding="utf-8",
    )
    task = parse_task(p)
    assert task.blocked_reason == BlockedReason.QUESTION
    assert task.split_from == "007"


def test_parse_task_defaults_no_blocked_reason(tmp_path: Path) -> None:
    p = tmp_path / "001.md"
    p.write_text(
        "---\nid: 1\ntitle: T\nstatus: ready\npipeline: default\n---\n",
        encoding="utf-8",
    )
    task = parse_task(p)
    assert task.blocked_reason is None
    assert task.split_from is None


# --- REST ------------------------------------------------------------------------


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


@pytest.fixture
def api(board_repo: Path) -> tuple[TestClient, BoardState]:
    app = FastAPI()
    app.include_router(conversations_router)
    state = BoardState()
    state.projects["demo"] = ProjectModel(name="demo", ssh_url="git@example.com:demo.git")
    app.dependency_overrides[get_board_context] = lambda: (state, board_repo)
    return TestClient(app), state


def _board_log(board_repo: Path) -> list[str]:
    out = subprocess.run(
        ["git", "log", "--format=%s"],
        cwd=str(board_repo),
        check=True,
        capture_output=True,
        text=True,
    )
    return out.stdout.strip().splitlines()


def test_create_conversation_lands_pending(
    api: tuple[TestClient, BoardState], board_repo: Path
) -> None:
    client, state = api
    resp = client.post(
        "/api/projects/demo/conversations",
        json={"title": "Adaptive feeds", "content": "We discussed feeds.", "source": "voice"},
    )
    assert resp.status_code == 200
    cid = resp.json()["conversation_id"]
    assert cid == "001"
    conversation = state.projects["demo"].conversations[cid]
    assert conversation.status == ConversationStatus.PENDING
    assert _board_log(board_repo)[0] == "[board:conversation] create demo/001"


def test_create_conversation_invalid_source(api: tuple[TestClient, BoardState]) -> None:
    client, _ = api
    resp = client.post(
        "/api/projects/demo/conversations", json={"title": "T", "source": "carrier-pigeon"}
    )
    assert resp.status_code == 422


def test_list_and_detail(api: tuple[TestClient, BoardState]) -> None:
    client, _ = api
    client.post("/api/projects/demo/conversations", json={"title": "A", "content": "aaa"})
    client.post("/api/projects/demo/conversations", json={"title": "B"})
    listing = client.get("/api/projects/demo/conversations").json()
    assert [c["conversation_id"] for c in listing] == ["001", "002"]
    detail = client.get("/api/projects/demo/conversations/001").json()
    assert detail["body"] == "aaa"
    assert detail["result_content"] is None


def test_status_transitions_enforced(
    api: tuple[TestClient, BoardState], board_repo: Path
) -> None:
    client, state = api
    client.post("/api/projects/demo/conversations", json={"title": "T"})

    # illegal jump pending → decomposed
    resp = client.patch(
        "/api/projects/demo/conversations/001/status", json={"status": "decomposed"}
    )
    assert resp.status_code == 409

    resp = client.patch(
        "/api/projects/demo/conversations/001/status", json={"status": "decomposing"}
    )
    assert resp.status_code == 200
    resp = client.patch(
        "/api/projects/demo/conversations/001/status",
        json={
            "status": "decomposed",
            "decomposed_into": ["feat-x"],
            "result_content": "## Summary\nDone.",
        },
    )
    assert resp.status_code == 200
    conversation = state.projects["demo"].conversations["001"]
    assert conversation.status == ConversationStatus.DECOMPOSED
    assert conversation.decomposed_into == ["feat-x"]

    detail = client.get("/api/projects/demo/conversations/001").json()
    assert detail["result_content"] == "## Summary\nDone."

    # terminal: no further transitions
    resp = client.patch(
        "/api/projects/demo/conversations/001/status", json={"status": "decomposing"}
    )
    assert resp.status_code == 409
    # each mutation was one board commit
    assert _board_log(board_repo)[:3] == [
        "[board:conversation] demo/001 → decomposed",
        "[board:conversation] demo/001 → decomposing",
        "[board:conversation] create demo/001",
    ]


def test_decomposing_can_return_to_pending(api: tuple[TestClient, BoardState]) -> None:
    client, state = api
    client.post("/api/projects/demo/conversations", json={"title": "T"})
    client.patch(
        "/api/projects/demo/conversations/001/status", json={"status": "decomposing"}
    )
    resp = client.patch(
        "/api/projects/demo/conversations/001/status",
        json={"status": "pending", "result_content": "decompose failed: rate limit"},
    )
    assert resp.status_code == 200
    assert (
        state.projects["demo"].conversations["001"].status == ConversationStatus.PENDING
    )


def test_pending_queue_ordering(board_repo: Path) -> None:
    import borealis.service.main as main_module
    from borealis.service.main import app

    state = BoardState()
    project = ProjectModel(name="demo", ssh_url="git@example.com:demo.git")
    state.projects["demo"] = project
    conversations_dir = board_repo / "projects" / "demo" / "conversations"
    for cid, created in (("001", "2026-06-12T11:00:00+00:00"),
                         ("002", "2026-06-12T10:00:00+00:00")):
        path = write_conversation_file(
            conversations_dir,
            cid,
            {"id": cid, "title": cid, "status": "pending", "created_at": created},
            "",
        )
        project.conversations[cid] = parse_conversation(path)
    # one decomposed conversation must not appear
    path = write_conversation_file(
        conversations_dir, "003", {"id": "003", "title": "x", "status": "decomposed"}, ""
    )
    project.conversations["003"] = parse_conversation(path)

    original = main_module._board_state
    main_module._board_state = state
    try:
        client = TestClient(app)
        pending = client.get("/api/conversations/pending").json()
    finally:
        main_module._board_state = original
    assert [c["conversation_id"] for c in pending] == ["002", "001"]
