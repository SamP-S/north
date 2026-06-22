"""Feature archive/unarchive must leave the board repo clean.

Regression: archiving staged only the new archived/ files, leaving the
active/ deletions unstaged — a permanently dirty board repo on which
`pull --rebase` (remote sync) fails forever. Found by the first live
approve (M4 smoke).
"""

import subprocess
from pathlib import Path

import frontmatter
import pytest
from borealis.service.api.deps import get_board_context
from borealis.service.api.features import router as features_router
from borealis.service.board.parser import parse_feature, parse_task
from borealis.service.models import BoardState, ProjectModel
from fastapi import FastAPI
from fastapi.testclient import TestClient


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


@pytest.fixture
def api(board_repo: Path) -> tuple[TestClient, BoardState]:
    feature_dir = (
        board_repo / "projects" / "demo" / "board" / "features" / "active" / "feat-x"
    )
    tasks_dir = feature_dir / "tasks"
    tasks_dir.mkdir(parents=True)
    feature_file = feature_dir / "_feature.md"
    frontmatter.dump(
        frontmatter.Post("desc", id="feat-x", title="Feat", status="review"),
        str(feature_file),
    )
    (feature_dir / "_feature.thread.md").write_text("# Thread\n", encoding="utf-8")
    task_file = tasks_dir / "001.md"
    frontmatter.dump(
        frontmatter.Post("body", id="001", title="T", status="done", pipeline="default"),
        str(task_file),
    )
    (tasks_dir / "001.result.md").write_text("## Handoff notes\n", encoding="utf-8")
    _run(board_repo, "add", "-A")
    _run(board_repo, "commit", "-m", "seed feature")

    app = FastAPI()
    app.include_router(features_router)
    state = BoardState()
    project = ProjectModel(name="demo", ssh_url="git@example.com:demo.git")
    state.projects["demo"] = project
    feature = parse_feature(feature_file)
    project.features["feat-x"] = feature
    feature.tasks["001"] = parse_task(task_file)
    app.dependency_overrides[get_board_context] = lambda: (state, board_repo)
    return TestClient(app), state


def _porcelain(board_repo: Path) -> str:
    return _run(board_repo, "status", "--porcelain", "-uall").strip()


def test_archive_leaves_repo_clean(
    api: tuple[TestClient, BoardState], board_repo: Path
) -> None:
    client, _ = api
    response = client.patch(
        "/api/projects/demo/features/feat-x/status", json={"status": "merged"}
    )
    assert response.status_code == 200
    assert _porcelain(board_repo) == ""
    archived = (
        board_repo / "projects" / "demo" / "board" / "features" / "archived" / "feat-x"
    )
    assert (archived / "_feature.md").exists()
    assert (archived / "tasks" / "001.result.md").exists()


def test_requeue_from_archive_leaves_repo_clean(
    api: tuple[TestClient, BoardState], board_repo: Path
) -> None:
    client, _ = api
    client.patch("/api/projects/demo/features/feat-x/status", json={"status": "merged"})
    response = client.post("/api/projects/demo/features/feat-x/requeue")
    assert response.status_code == 200
    assert _porcelain(board_repo) == ""
    active = (
        board_repo / "projects" / "demo" / "board" / "features" / "active" / "feat-x"
    )
    assert (active / "_feature.md").exists()
    assert parse_feature(active / "_feature.md").status == "open"
    assert parse_task(active / "tasks" / "001.md").status == "ready"
