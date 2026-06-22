"""Tests for server-enforced draft + promotion + status transition gating."""

import subprocess
from pathlib import Path

import pytest
from borealis.service.api.deps import get_board_context
from borealis.service.api.features import router as features_router
from borealis.service.api.tasks import router as tasks_router
from borealis.service.board.parser import parse_feature, parse_task
from borealis.service.models import (
    BoardState,
    FeatureStatus,
    ProjectModel,
    TaskStatus,
)
from fastapi import FastAPI
from fastapi.testclient import TestClient


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
    app.include_router(tasks_router)
    app.include_router(features_router)
    state = BoardState()
    state.projects["demo"] = ProjectModel(name="demo", ssh_url="git@example.com:demo.git")
    app.dependency_overrides[get_board_context] = lambda: (state, board_repo)
    return TestClient(app), state


def test_created_feature_and_task_land_draft(api: tuple[TestClient, BoardState]) -> None:
    client, state = api
    resp = client.post("/api/projects/demo/features", json={"title": "Feat X"})
    assert resp.status_code == 200
    feature = state.projects["demo"].features["feat-x"]
    assert feature.status == FeatureStatus.DRAFT
    assert parse_feature(feature.feature_path).status == FeatureStatus.DRAFT

    resp = client.post(
        "/api/projects/demo/features/feat-x/tasks",
        json={"title": "T", "pipeline": "default"},
    )
    assert resp.status_code == 200
    task = feature.tasks["001"]
    assert task.status == TaskStatus.DRAFT
    assert parse_task(task.task_path).status == TaskStatus.DRAFT


def test_draft_cannot_jump_via_status_patch(api: tuple[TestClient, BoardState]) -> None:
    client, _ = api
    client.post("/api/projects/demo/features", json={"title": "Feat X"})
    client.post(
        "/api/projects/demo/features/feat-x/tasks",
        json={"title": "T", "pipeline": "default"},
    )
    for target in ("ready", "queued", "done"):
        resp = client.patch(
            "/api/projects/demo/features/feat-x/tasks/001/status",
            json={"status": target},
        )
        assert resp.status_code == 409, target
    resp = client.patch(
        "/api/projects/demo/features/feat-x/status", json={"status": "open"}
    )
    assert resp.status_code == 409


def test_task_promote_requires_feature_promotion_first(
    api: tuple[TestClient, BoardState],
) -> None:
    client, state = api
    client.post("/api/projects/demo/features", json={"title": "Feat X"})
    client.post(
        "/api/projects/demo/features/feat-x/tasks",
        json={"title": "T", "pipeline": "default"},
    )

    resp = client.post("/api/projects/demo/features/feat-x/tasks/001/promote")
    assert resp.status_code == 409  # feature still draft

    resp = client.post("/api/projects/demo/features/feat-x/promote")
    assert resp.status_code == 200
    assert state.projects["demo"].features["feat-x"].status == FeatureStatus.OPEN

    resp = client.post("/api/projects/demo/features/feat-x/tasks/001/promote")
    assert resp.status_code == 200
    task = state.projects["demo"].features["feat-x"].tasks["001"]
    assert task.status == TaskStatus.READY
    assert parse_task(task.task_path).status == TaskStatus.READY


def test_promote_non_draft_409(api: tuple[TestClient, BoardState]) -> None:
    client, _ = api
    client.post("/api/projects/demo/features", json={"title": "Feat X"})
    client.post(
        "/api/projects/demo/features/feat-x/tasks",
        json={"title": "T", "pipeline": "default"},
    )
    client.post("/api/projects/demo/features/feat-x/promote")
    assert client.post("/api/projects/demo/features/feat-x/promote").status_code == 409
    client.post("/api/projects/demo/features/feat-x/tasks/001/promote")
    resp = client.post("/api/projects/demo/features/feat-x/tasks/001/promote")
    assert resp.status_code == 409


def test_task_transition_table(api: tuple[TestClient, BoardState]) -> None:
    client, state = api
    client.post("/api/projects/demo/features", json={"title": "Feat X"})
    client.post(
        "/api/projects/demo/features/feat-x/tasks",
        json={"title": "T", "pipeline": "default"},
    )
    client.post("/api/projects/demo/features/feat-x/promote")
    client.post("/api/projects/demo/features/feat-x/tasks/001/promote")
    base = "/api/projects/demo/features/feat-x/tasks/001/status"

    # ready → done illegal
    assert client.patch(base, json={"status": "done"}).status_code == 409
    # ready → queued → in_progress → done legal
    assert client.patch(base, json={"status": "queued"}).status_code == 200
    assert client.patch(base, json={"status": "in_progress"}).status_code == 200
    assert client.patch(base, json={"status": "done"}).status_code == 200
    # done → ready (manual re-run) legal; done → in_progress illegal
    assert client.patch(base, json={"status": "in_progress"}).status_code == 409
    assert client.patch(base, json={"status": "ready"}).status_code == 200
    task = state.projects["demo"].features["feat-x"].tasks["001"]
    assert task.status == TaskStatus.READY


def test_decomposed_from_round_trip(api: tuple[TestClient, BoardState]) -> None:
    client, state = api
    resp = client.post(
        "/api/projects/demo/features",
        json={"title": "Feat X", "decomposed_from": "001"},
    )
    assert resp.status_code == 200
    feature = state.projects["demo"].features["feat-x"]
    assert feature.decomposed_from == "001"
    assert parse_feature(feature.feature_path).decomposed_from == "001"

    resp = client.post(
        "/api/projects/demo/features/feat-x/tasks",
        json={"title": "T", "pipeline": "default", "decomposed_from": "001"},
    )
    assert resp.status_code == 200
    task = feature.tasks["001"]
    assert task.decomposed_from == "001"
    detail = client.get("/api/projects/demo/features/feat-x/tasks/001").json()
    assert detail["decomposed_from"] == "001"
    detail = client.get("/api/projects/demo/features/feat-x").json()
    assert detail["decomposed_from"] == "001"


def test_put_task_cannot_smuggle_status(api: tuple[TestClient, BoardState]) -> None:
    client, _ = api
    client.post("/api/projects/demo/features", json={"title": "Feat X"})
    client.post(
        "/api/projects/demo/features/feat-x/tasks",
        json={"title": "T", "pipeline": "default"},
    )
    resp = client.put(
        "/api/projects/demo/features/feat-x/tasks/001",
        json={"title": "T2", "pipeline": "default", "status": "done"},
    )
    assert resp.status_code == 409
    # same-status PUT still allowed (content edit)
    resp = client.put(
        "/api/projects/demo/features/feat-x/tasks/001",
        json={"title": "T2", "pipeline": "default", "status": "draft", "body": "new"},
    )
    assert resp.status_code == 200
