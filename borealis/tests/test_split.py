"""Tests for the task split verb: atomic relink + superseded parent."""

import subprocess
from pathlib import Path

import frontmatter
import pytest
from borealis.service.api.deps import get_board_context
from borealis.service.api.tasks import router as tasks_router
from borealis.service.board.parser import parse_task
from borealis.service.models import (
    BoardState,
    FeatureModel,
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


def _add_task(
    board_repo: Path,
    feature: FeatureModel,
    task_id: str,
    status: str = "ready",
    depends_on: list[str] | None = None,
) -> None:
    tasks_dir = (
        board_repo / "projects" / "demo" / "board" / "features" / "active"
        / feature.feature_id / "tasks"
    )
    tasks_dir.mkdir(parents=True, exist_ok=True)
    path = tasks_dir / f"{task_id}.md"
    frontmatter.dump(
        frontmatter.Post(
            f"body {task_id}",
            id=task_id,
            title=f"Task {task_id}",
            status=status,
            pipeline="default",
            depends_on=depends_on or [],
        ),
        str(path),
    )
    feature.tasks[task_id] = parse_task(path)


@pytest.fixture
def api(board_repo: Path) -> tuple[TestClient, BoardState]:
    app = FastAPI()
    app.include_router(tasks_router)
    state = BoardState()
    project = ProjectModel(name="demo", ssh_url="git@example.com:demo.git")
    state.projects["demo"] = project
    feature_dir = (
        board_repo / "projects" / "demo" / "board" / "features" / "active" / "feat-x"
    )
    feature_dir.mkdir(parents=True)
    feature_path = feature_dir / "_feature.md"
    frontmatter.dump(
        frontmatter.Post("desc", id="feat-x", title="Feat", status="open"),
        str(feature_path),
    )
    project.features["feat-x"] = FeatureModel(
        feature_id="feat-x",
        title="Feat",
        status=FeatureStatus.OPEN,
        feature_path=feature_path,
    )
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


def test_split_relinks_graph_in_one_commit(
    api: tuple[TestClient, BoardState], board_repo: Path
) -> None:
    client, state = api
    feature = state.projects["demo"].features["feat-x"]
    _add_task(board_repo, feature, "001", status="done")
    _add_task(board_repo, feature, "002", status="ready", depends_on=["001"])
    _add_task(board_repo, feature, "003", status="ready", depends_on=["002"])
    _run(board_repo, "add", "-A")
    _run(board_repo, "commit", "-m", "seed")
    commits_before = len(_board_log(board_repo))

    resp = client.post(
        "/api/projects/demo/features/feat-x/tasks/002/split",
        json={
            "tasks": [
                {"title": "Part A", "body": "do A"},
                {"title": "Part B", "body": "do B", "pipeline": "local"},
            ]
        },
    )
    assert resp.status_code == 200
    assert resp.json() == {"message": "ok", "created": ["004", "005"], "superseded": "002"}

    # one board commit
    log = _board_log(board_repo)
    assert len(log) == commits_before + 1
    assert log[0] == "[board:task] split demo/feat-x/002 → 004, 005"

    # children inherit parent's deps + split_from; pipeline default/override
    child_a = feature.tasks["004"]
    child_b = feature.tasks["005"]
    assert child_a.depends_on == ["001"]
    assert child_b.depends_on == ["001"]
    assert child_a.split_from == "002"
    assert child_a.pipeline == "default"
    assert child_b.pipeline == "local"
    # promoted parent → children land ready
    assert child_a.status == TaskStatus.READY

    # dependent re-pointed to all children
    assert feature.tasks["003"].depends_on == ["004", "005"]
    assert parse_task(feature.tasks["003"].task_path).depends_on == ["004", "005"]

    # parent superseded, kept on disk
    assert feature.tasks["002"].status == TaskStatus.SUPERSEDED
    assert parse_task(feature.tasks["002"].task_path).status == TaskStatus.SUPERSEDED


def test_split_draft_parent_creates_draft_children(
    api: tuple[TestClient, BoardState], board_repo: Path
) -> None:
    client, state = api
    feature = state.projects["demo"].features["feat-x"]
    _add_task(board_repo, feature, "001", status="draft")
    resp = client.post(
        "/api/projects/demo/features/feat-x/tasks/001/split",
        json={"tasks": [{"title": "Part A"}]},
    )
    assert resp.status_code == 200
    assert feature.tasks["002"].status == TaskStatus.DRAFT


def test_split_guards(api: tuple[TestClient, BoardState], board_repo: Path) -> None:
    client, state = api
    feature = state.projects["demo"].features["feat-x"]
    _add_task(board_repo, feature, "001", status="done")
    _add_task(board_repo, feature, "002", status="in_progress")

    for task_id in ("001", "002"):
        resp = client.post(
            f"/api/projects/demo/features/feat-x/tasks/{task_id}/split",
            json={"tasks": [{"title": "x"}]},
        )
        assert resp.status_code == 409, task_id

    resp = client.post(
        "/api/projects/demo/features/feat-x/tasks/001/split", json={"tasks": []}
    )
    assert resp.status_code == 422

    resp = client.post(
        "/api/projects/demo/features/feat-x/tasks/999/split",
        json={"tasks": [{"title": "x"}]},
    )
    assert resp.status_code == 404


def test_split_superseded_parent_rejected(
    api: tuple[TestClient, BoardState], board_repo: Path
) -> None:
    client, state = api
    feature = state.projects["demo"].features["feat-x"]
    _add_task(board_repo, feature, "001", status="ready")
    client.post(
        "/api/projects/demo/features/feat-x/tasks/001/split",
        json={"tasks": [{"title": "x"}]},
    )
    resp = client.post(
        "/api/projects/demo/features/feat-x/tasks/001/split",
        json={"tasks": [{"title": "y"}]},
    )
    assert resp.status_code == 409
