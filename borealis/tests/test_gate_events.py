"""Gate events reach the notifier: conversation_decomposed, task_failed,
feature_review (emitted when the aurora/brief note lands on a review feature)."""

import subprocess
import threading
from collections.abc import Iterator
from pathlib import Path

import frontmatter
import pytest
from borealis.service import events
from borealis.service.api.comments import router as comments_router
from borealis.service.api.conversations import router as conversations_router
from borealis.service.api.deps import get_board_context
from borealis.service.api.tasks import router as tasks_router
from borealis.service.board.parser import parse_conversation, parse_feature, parse_task
from borealis.service.models import BoardState, ProjectModel
from borealis.service.notify import Notifier
from fastapi import FastAPI
from fastapi.testclient import TestClient


class FakeTransport:
    def __init__(self) -> None:
        self.sent: list[str] = []
        self.lock = threading.Lock()

    def send(self, text: str) -> None:
        with self.lock:
            self.sent.append(text)


@pytest.fixture
def transport() -> Iterator[FakeTransport]:
    fake = FakeTransport()
    events.set_notifier(Notifier(fake))
    yield fake
    events.set_notifier(None)


def _drain() -> None:
    events.get_notifier().drain()


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
        frontmatter.Post(
            "body", id="001", title="T", status="in_progress", pipeline="default"
        ),
        str(task_file),
    )
    conv_dir = board_repo / "projects" / "demo" / "conversations"
    conv_dir.mkdir(parents=True)
    conv_file = conv_dir / "001.md"
    frontmatter.dump(
        frontmatter.Post("intake", id="001", title="Conv", status="decomposing"),
        str(conv_file),
    )

    app = FastAPI()
    app.include_router(tasks_router)
    app.include_router(comments_router)
    app.include_router(conversations_router)
    state = BoardState()
    project = ProjectModel(name="demo", ssh_url="git@example.com:demo.git")
    state.projects["demo"] = project
    feature = parse_feature(feature_file)
    project.features["feat-x"] = feature
    feature.tasks["001"] = parse_task(task_file)
    project.conversations["001"] = parse_conversation(conv_file)
    app.dependency_overrides[get_board_context] = lambda: (state, board_repo)
    return TestClient(app), state


def test_conversation_decomposed_emits(
    api: tuple[TestClient, BoardState], transport: FakeTransport
) -> None:
    client, _ = api
    response = client.patch(
        "/api/projects/demo/conversations/001/status",
        json={"status": "decomposed", "decomposed_into": ["feat-y", "feat-y/001"]},
    )
    assert response.status_code == 200
    _drain()
    assert len(transport.sent) == 1
    assert transport.sent[0].startswith("[conversation_decomposed]")
    assert "feat-y, feat-y/001" in transport.sent[0]


def test_failed_decompose_back_to_pending_does_not_emit(
    api: tuple[TestClient, BoardState], transport: FakeTransport
) -> None:
    client, _ = api
    response = client.patch(
        "/api/projects/demo/conversations/001/status", json={"status": "pending"}
    )
    assert response.status_code == 200
    _drain()
    assert transport.sent == []


def test_task_failed_emits(
    api: tuple[TestClient, BoardState], transport: FakeTransport
) -> None:
    client, _ = api
    response = client.patch(
        "/api/projects/demo/features/feat-x/tasks/001/status",
        json={"status": "failed"},
    )
    assert response.status_code == 200
    _drain()
    assert transport.sent == ["[task_failed] feature=feat-x project=demo task=001"]


def test_task_done_does_not_emit_task_failed(
    api: tuple[TestClient, BoardState], transport: FakeTransport
) -> None:
    client, _ = api
    response = client.patch(
        "/api/projects/demo/features/feat-x/tasks/001/status",
        json={"status": "done"},
    )
    assert response.status_code == 200
    _drain()
    assert not any(t.startswith("[task_failed]") for t in transport.sent)


def test_brief_note_on_review_feature_emits_feature_review(
    api: tuple[TestClient, BoardState], transport: FakeTransport
) -> None:
    client, state = api
    # finishing the only task flips the feature to review
    client.patch(
        "/api/projects/demo/features/feat-x/tasks/001/status",
        json={"status": "done"},
    )
    response = client.post(
        "/api/projects/demo/features/feat-x/comments",
        json={
            "kind": "note",
            "author": "aurora/brief",
            "text": "feat-x ready: 1 task, +10/-2, gates green\n\nDetails...\n",
        },
    )
    assert response.status_code == 200
    _drain()
    assert transport.sent == [
        "[feature_review] feat-x ready: 1 task, +10/-2, gates green"
    ]


def test_human_note_on_review_feature_does_not_emit(
    api: tuple[TestClient, BoardState], transport: FakeTransport
) -> None:
    client, _ = api
    client.patch(
        "/api/projects/demo/features/feat-x/tasks/001/status",
        json={"status": "done"},
    )
    response = client.post(
        "/api/projects/demo/features/feat-x/comments",
        json={"kind": "note", "author": "sam", "text": "looks fine"},
    )
    assert response.status_code == 200
    _drain()
    assert transport.sent == []


def test_brief_note_on_non_review_feature_does_not_emit(
    api: tuple[TestClient, BoardState], transport: FakeTransport
) -> None:
    client, _ = api
    response = client.post(
        "/api/projects/demo/features/feat-x/comments",
        json={"kind": "note", "author": "aurora/brief", "text": "early note"},
    )
    assert response.status_code == 200
    _drain()
    assert transport.sent == []
