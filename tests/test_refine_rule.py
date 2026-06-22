"""Refine rule: creating a task on a feature in review reverts it to
in_progress, task write + feature flip in one board commit."""

import subprocess
from pathlib import Path

import frontmatter
import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from north.service.api.deps import get_board_context
from north.service.api.tasks import router as tasks_router
from north.service.board.parser import parse_feature, parse_task
from north.service.models import BoardState, FeatureStatus, ProjectModel


def _run(cwd: Path, *args: str) -> str:
    out = subprocess.run(
        ["git", *args], cwd=str(cwd), check=True, capture_output=True, text=True
    )
    return out.stdout


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


def _api(board_repo: Path, feature_status: str) -> tuple[TestClient, BoardState]:
    feature_dir = (
        board_repo / "projects" / "demo" / "board" / "features" / "active" / "feat-x"
    )
    tasks_dir = feature_dir / "tasks"
    tasks_dir.mkdir(parents=True)
    feature_file = feature_dir / "_feature.md"
    frontmatter.dump(
        frontmatter.Post("desc", id="feat-x", title="Feat", status=feature_status),
        str(feature_file),
    )
    task_file = tasks_dir / "001.md"
    frontmatter.dump(
        frontmatter.Post(
            "body", id="001", title="T", status="done", pipeline="default"
        ),
        str(task_file),
    )

    app = FastAPI()
    app.include_router(tasks_router)
    state = BoardState()
    project = ProjectModel(name="demo", ssh_url="git@example.com:demo.git")
    state.projects["demo"] = project
    feature = parse_feature(feature_file)
    project.features["feat-x"] = feature
    feature.tasks["001"] = parse_task(task_file)
    app.dependency_overrides[get_board_context] = lambda: (state, board_repo)
    return TestClient(app), state


def test_create_on_review_feature_reverts_to_in_progress(board_repo: Path) -> None:
    client, state = _api(board_repo, "review")
    response = client.post(
        "/api/projects/demo/features/feat-x/tasks",
        json={"title": "Refinement", "pipeline": "default"},
    )
    assert response.status_code == 200
    feature = state.projects["demo"].features["feat-x"]
    assert feature.status == FeatureStatus.IN_PROGRESS
    assert parse_feature(feature.feature_path).status == FeatureStatus.IN_PROGRESS

    # one commit covers both the new task and the feature flip
    subject = _run(board_repo, "log", "-1", "--format=%s").strip()
    assert subject == "[board:task] create demo/feat-x/002 (review → in_progress)"
    changed = _run(board_repo, "show", "--name-only", "--format=", "HEAD").split()
    assert any(p.endswith("002.md") for p in changed)
    assert any(p.endswith("_feature.md") for p in changed)


def test_refined_feature_returns_to_review_when_task_done(board_repo: Path) -> None:
    client, state = _api(board_repo, "review")
    client.post(
        "/api/projects/demo/features/feat-x/tasks",
        json={"title": "Refinement", "pipeline": "default"},
    )
    client.post("/api/projects/demo/features/feat-x/tasks/002/promote")
    for status in ("queued", "in_progress", "done"):
        response = client.patch(
            "/api/projects/demo/features/feat-x/tasks/002/status",
            json={"status": status},
        )
        assert response.status_code == 200, response.text
    assert state.projects["demo"].features["feat-x"].status == FeatureStatus.REVIEW


def test_create_on_open_feature_does_not_flip(board_repo: Path) -> None:
    client, state = _api(board_repo, "open")
    response = client.post(
        "/api/projects/demo/features/feat-x/tasks",
        json={"title": "More work", "pipeline": "default"},
    )
    assert response.status_code == 200
    assert state.projects["demo"].features["feat-x"].status == FeatureStatus.OPEN
    subject = _run(board_repo, "log", "-1", "--format=%s").strip()
    assert subject == "[board:task] create demo/feat-x/002"
