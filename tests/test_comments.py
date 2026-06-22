"""Tests for comment threads: append-only writer + lenient parser + REST."""

import subprocess
from datetime import UTC, datetime
from pathlib import Path

import frontmatter
import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from north.service.api.comments import router as comments_router
from north.service.api.deps import get_board_context
from north.service.board.parser import parse_feature, parse_task, parse_thread
from north.service.board.writer import append_thread_entry
from north.service.models import BoardState, ProjectModel, ThreadEntryKind

AT = "2026-06-12T10:00:00+00:00"


def test_missing_thread_is_empty(tmp_path: Path) -> None:
    assert parse_thread(tmp_path / "001.thread.md") == []


def test_append_and_parse_round_trip(tmp_path: Path) -> None:
    path = tmp_path / "001.thread.md"
    append_thread_entry(path, "question", "north/implement", AT, "Is the API versioned?")
    append_thread_entry(path, "answer", "sam", AT, "No, keep it flat.")
    entries = parse_thread(path)
    assert len(entries) == 2
    assert entries[0].kind == ThreadEntryKind.QUESTION
    assert entries[0].author == "north/implement"
    assert entries[0].at == datetime(2026, 6, 12, 10, 0, tzinfo=UTC)
    assert entries[0].text == "Is the API versioned?"
    assert entries[1].kind == ThreadEntryKind.ANSWER
    assert entries[1].text == "No, keep it flat."


def test_multiline_entry_body_preserved(tmp_path: Path) -> None:
    path = tmp_path / "001.thread.md"
    append_thread_entry(path, "note", "sam", AT, "line one\n\nline three")
    entries = parse_thread(path)
    assert entries[0].text == "line one\n\nline three"


def test_hand_edited_garbage_is_skipped(tmp_path: Path) -> None:
    path = tmp_path / "001.thread.md"
    path.write_text(
        (
            "random preamble\n\n"
            "## [note] sam — not-a-timestamp\n\nlost entry\n\n"
            f"## [shout] sam — {AT}\n\nbad kind\n\n"
            f"## [note] sam — {AT}\n\ngood entry\n"
        ),
        encoding="utf-8",
    )
    entries = parse_thread(path)
    assert len(entries) == 1
    assert entries[0].text == "good entry"


def test_append_to_hand_edited_file(tmp_path: Path) -> None:
    path = tmp_path / "001.thread.md"
    path.write_text("# Thread\n\nsome prose\n", encoding="utf-8")
    append_thread_entry(path, "note", "sam", AT, "appended")
    entries = parse_thread(path)
    assert len(entries) == 1
    assert entries[0].text == "appended"


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
    feature_dir = (
        board_repo / "projects" / "demo" / "board" / "features" / "active" / "feat-x"
    )
    tasks_dir = feature_dir / "tasks"
    tasks_dir.mkdir(parents=True)
    feature_file = feature_dir / "_feature.md"
    frontmatter.dump(
        frontmatter.Post("desc", id="feat-x", title="Feat", status="open"),
        str(feature_file),
    )
    task_file = tasks_dir / "001.md"
    frontmatter.dump(
        frontmatter.Post("body", id="001", title="T", status="ready", pipeline="default"),
        str(task_file),
    )

    app = FastAPI()
    app.include_router(comments_router)
    state = BoardState()
    project = ProjectModel(name="demo", ssh_url="git@example.com:demo.git")
    state.projects["demo"] = project
    feature = parse_feature(feature_file)
    project.features["feat-x"] = feature
    feature.tasks["001"] = parse_task(task_file)
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


def test_task_comments_round_trip(
    api: tuple[TestClient, BoardState], board_repo: Path
) -> None:
    client, _ = api
    base = "/api/projects/demo/features/feat-x/tasks/001/comments"
    assert client.get(base).json() == []
    resp = client.post(
        base, json={"kind": "question", "author": "north/implement", "text": "Versioned?"}
    )
    assert resp.status_code == 200
    resp = client.post(base, json={"kind": "answer", "author": "sam", "text": "No."})
    assert resp.status_code == 200

    comments = client.get(base).json()
    assert [c["kind"] for c in comments] == ["question", "answer"]
    assert comments[0]["author"] == "north/implement"
    assert comments[1]["text"] == "No."
    assert _board_log(board_repo)[0] == "[board:comment] demo/feat-x/001 [answer] by sam"


def test_feature_comments_round_trip(
    api: tuple[TestClient, BoardState], board_repo: Path
) -> None:
    client, _ = api
    base = "/api/projects/demo/features/feat-x/comments"
    assert client.get(base).json() == []
    resp = client.post(base, json={"kind": "note", "author": "sam", "text": "Review remark."})
    assert resp.status_code == 200
    comments = client.get(base).json()
    assert len(comments) == 1
    assert comments[0]["text"] == "Review remark."
    thread = (
        board_repo / "projects" / "demo" / "board" / "features" / "active" / "feat-x"
        / "_feature.thread.md"
    )
    assert thread.exists()
    assert _board_log(board_repo)[0] == "[board:comment] demo/feat-x [note] by sam"


def test_invalid_kind_rejected(api: tuple[TestClient, BoardState]) -> None:
    client, _ = api
    resp = client.post(
        "/api/projects/demo/features/feat-x/tasks/001/comments",
        json={"kind": "shout", "author": "sam", "text": "x"},
    )
    assert resp.status_code == 422


def test_unknown_task_404(api: tuple[TestClient, BoardState]) -> None:
    client, _ = api
    resp = client.get("/api/projects/demo/features/feat-x/tasks/999/comments")
    assert resp.status_code == 404


def _block_on_question(api: tuple[TestClient, BoardState]) -> None:
    from north.service.board.writer import update_task_frontmatter
    from north.service.models import BlockedReason, TaskStatus

    _, state = api
    task = state.projects["demo"].features["feat-x"].tasks["001"]
    update_task_frontmatter(
        task.task_path, {"status": "blocked", "blocked_reason": "question"}
    )
    task.status = TaskStatus.BLOCKED
    task.blocked_reason = BlockedReason.QUESTION


def test_answer_flips_question_blocked_task_to_ready(
    api: tuple[TestClient, BoardState], board_repo: Path
) -> None:
    from north.service.models import TaskStatus

    client, state = api
    _block_on_question(api)
    commits_before = len(_board_log(board_repo))

    resp = client.post(
        "/api/projects/demo/features/feat-x/tasks/001/comments",
        json={"kind": "answer", "author": "sam", "text": "Use v2."},
    )
    assert resp.status_code == 200
    assert resp.json()["task_status"] == "ready"

    task = state.projects["demo"].features["feat-x"].tasks["001"]
    assert task.status == TaskStatus.READY
    assert task.blocked_reason is None
    assert parse_task(task.task_path).status == TaskStatus.READY
    assert parse_task(task.task_path).blocked_reason is None
    # thread append + status flip land as one board commit
    log = _board_log(board_repo)
    assert len(log) == commits_before + 1
    assert log[0].endswith("(answered → ready)")


def test_note_does_not_unblock(api: tuple[TestClient, BoardState]) -> None:
    from north.service.models import TaskStatus

    client, state = api
    _block_on_question(api)
    resp = client.post(
        "/api/projects/demo/features/feat-x/tasks/001/comments",
        json={"kind": "note", "author": "sam", "text": "Thinking about it."},
    )
    assert resp.status_code == 200
    task = state.projects["demo"].features["feat-x"].tasks["001"]
    assert task.status == TaskStatus.BLOCKED


def test_answer_on_infra_blocked_does_not_unblock(
    api: tuple[TestClient, BoardState],
) -> None:
    from north.service.board.writer import update_task_frontmatter
    from north.service.models import BlockedReason, TaskStatus

    client, state = api
    task = state.projects["demo"].features["feat-x"].tasks["001"]
    update_task_frontmatter(
        task.task_path, {"status": "blocked", "blocked_reason": "infra"}
    )
    task.status = TaskStatus.BLOCKED
    task.blocked_reason = BlockedReason.INFRA
    resp = client.post(
        "/api/projects/demo/features/feat-x/tasks/001/comments",
        json={"kind": "answer", "author": "sam", "text": "irrelevant"},
    )
    assert resp.status_code == 200
    assert task.status == TaskStatus.BLOCKED
