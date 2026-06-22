"""Tests for the project update (PATCH /api/projects/{project}) endpoint."""

from pathlib import Path

import pytest
from fastapi.testclient import TestClient

from north.service import main
from north.service.board.parser import parse_projects_yaml
from north.service.board.writer import write_projects_yaml
from north.service.models import BoardState, ProjectModel


@pytest.fixture
def client(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> TestClient:
    proj = ProjectModel(
        name="app", ssh_url="git@h:me/app.git", base_branch="main", auto_merge=False
    )
    write_projects_yaml(tmp_path, {"app": proj})
    monkeypatch.setattr(main, "_BOARD_PATH", tmp_path)
    monkeypatch.setattr(main, "_board_state", BoardState(projects={"app": proj}))
    monkeypatch.setattr(main, "_supervisor", None)
    monkeypatch.setattr(main, "commit_and_push_board", lambda *a, **k: None)
    # TestClient without `with` does not trigger the lifespan, preserving the
    # monkeypatched module state above.
    return TestClient(main.app)


def test_update_base_branch(client: TestClient) -> None:
    resp = client.patch("/api/projects/app", json={"base_branch": "develop"})
    assert resp.status_code == 200
    data = resp.json()
    assert data["base_branch"] == "develop"
    assert data["auto_merge"] is False
    assert main._current_board_state().projects["app"].base_branch == "develop"


def test_update_auto_merge(client: TestClient) -> None:
    resp = client.patch("/api/projects/app", json={"auto_merge": True})
    assert resp.status_code == 200
    assert resp.json()["auto_merge"] is True
    # untouched field is preserved
    assert resp.json()["base_branch"] == "main"


def test_update_persists_to_yaml(client: TestClient, tmp_path: Path) -> None:
    client.patch("/api/projects/app", json={"base_branch": "develop", "auto_merge": True})
    projects = parse_projects_yaml(tmp_path / "projects.yaml")
    assert projects["app"].base_branch == "develop"
    assert projects["app"].auto_merge is True


def test_update_unknown_project_404(client: TestClient) -> None:
    resp = client.patch("/api/projects/nope", json={"base_branch": "x"})
    assert resp.status_code == 404


def test_update_no_fields_400(client: TestClient) -> None:
    resp = client.patch("/api/projects/app", json={})
    assert resp.status_code == 400
