"""Tests for the North API endpoints."""

import subprocess
from pathlib import Path
from unittest.mock import patch

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from north.service.api.deps import get_board_context, set_board_context
from north.service.api.features import router as features_router
from north.service.api.tasks import router as tasks_router
from north.service.models import (
    BoardState,
    FeatureModel,
    FeatureStatus,
    ProjectModel,
    TaskModel,
    TaskStatus,
)


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
    set_board_context(lambda: state, board_repo)
    app.dependency_overrides[get_board_context] = lambda: (state, board_repo)

    return TestClient(app), state


def _ensure_project(state: BoardState, project: str = "demo") -> ProjectModel:
    project_model = state.projects.get(project)
    if project_model is None:
        project_model = ProjectModel(name=project, ssh_url="git@example.com:demo.git")
        state.projects[project] = project_model
    return project_model


def _ensure_feature(
    state: BoardState, project: str, feature: FeatureModel
) -> FeatureModel:
    project_model = _ensure_project(state, project)
    project_model.features[feature.feature_id] = feature
    return feature


def _make_task(
    board_repo: Path, task_id: str = "001", status: str = "queued"
) -> tuple[TaskModel, Path]:
    tasks_dir = (
        board_repo
        / "projects" / "demo"
        / "board" / "features" / "active" / "feat-x"
        / "tasks"
    )
    tasks_dir.mkdir(parents=True, exist_ok=True)
    path = tasks_dir / f"{task_id}.md"
    path.write_text(
        f"---\nid: {task_id}\ntitle: Task {task_id}\nstatus: {status}\n"
        "pipeline: default\n---\n\nDo the thing.\n",
        encoding="utf-8",
    )
    _run(board_repo, "add", str(path.relative_to(board_repo)))
    _run(board_repo, "commit", "-m", f"add task {task_id}")
    task = TaskModel(
        task_id=task_id,
        title=f"Task {task_id}",
        status=TaskStatus(status),
        pipeline="default",
        task_path=path,
        body="Do the thing.",
    )
    return task, path


def _make_feature(
    board_repo: Path, feature_id: str = "feat-x", status: str = "in_progress"
) -> tuple[FeatureModel, Path]:
    feature_dir = (
        board_repo
        / "projects" / "demo"
        / "board" / "features" / "active" / feature_id
    )
    feature_dir.mkdir(parents=True, exist_ok=True)
    path = feature_dir / "_feature.md"
    path.write_text(
        f"---\nid: {feature_id}\ntitle: Feature {feature_id}\nstatus: {status}\n"
        "---\n\nBody.\n",
        encoding="utf-8",
    )
    _run(board_repo, "add", str(path.relative_to(board_repo)))
    _run(board_repo, "commit", "-m", f"add feature {feature_id}")
    feature = FeatureModel(
        feature_id=feature_id,
        title=f"Feature {feature_id}",
        status=FeatureStatus(status),
        feature_path=path,
    )
    return feature, path


# --- GET /api/tasks ----------------------------------------------------------


def test_get_task_returns_data(api: tuple[TestClient, BoardState], board_repo: Path) -> None:
    client, state = api
    task, _ = _make_task(board_repo)
    feature, _ = _make_feature(board_repo)
    feature.tasks[task.task_id] = task
    _ensure_feature(state, "demo", feature)

    resp = client.get("/api/projects/demo/features/feat-x/tasks/001")
    assert resp.status_code == 200
    data = resp.json()
    assert data["task_id"] == "001"
    assert data["status"] == "queued"
    assert data["body"] == "Do the thing."


def test_get_task_404_unknown(api: tuple[TestClient, BoardState], board_repo: Path) -> None:
    client, state = api
    feature, _ = _make_feature(board_repo)
    _ensure_feature(state, "demo", feature)
    assert client.get("/api/projects/demo/features/feat-x/tasks/999").status_code == 404


def test_get_task_404_unknown_feature(api: tuple[TestClient, BoardState]) -> None:
    client, state = api
    _ensure_project(state, "demo")
    assert client.get("/api/projects/demo/features/feat-x/tasks/999").status_code == 404


def test_get_task_404_unknown_project(api: tuple[TestClient, BoardState]) -> None:
    client, _ = api
    assert client.get("/api/projects/demo/features/feat-x/tasks/999").status_code == 404


# --- PATCH /api/tasks status -------------------------------------------------


def test_patch_task_status(api: tuple[TestClient, BoardState], board_repo: Path) -> None:
    client, state = api
    task, _ = _make_task(board_repo, status="in_progress")
    feature, _ = _make_feature(board_repo)
    feature.tasks[task.task_id] = task
    _ensure_feature(state, "demo", feature)

    with patch("north.service.api.tasks.commit_and_push_board"):
        resp = client.patch(
            "/api/projects/demo/features/feat-x/tasks/001/status",
            json={"status": "done"},
        )

    assert resp.status_code == 200
    assert feature.tasks["001"].status == TaskStatus.DONE


def test_patch_task_writes_result_file(
    api: tuple[TestClient, BoardState], board_repo: Path
) -> None:
    client, state = api
    task, path = _make_task(board_repo, status="in_progress")
    feature, _ = _make_feature(board_repo)
    feature.tasks[task.task_id] = task
    _ensure_feature(state, "demo", feature)

    with patch("north.service.api.tasks.commit_and_push_board"):
        client.patch(
            "/api/projects/demo/features/feat-x/tasks/001/status",
            json={"status": "done", "result_content": "## Output\nall good"},
        )

    result_path = path.parent / "001.result.md"
    assert result_path.exists()
    assert "all good" in result_path.read_text()


def test_patch_blocked_stamps_reason_and_requeue_clears_it(
    api: tuple[TestClient, BoardState], board_repo: Path
) -> None:
    from north.service.board.parser import parse_task
    from north.service.models import BlockedReason

    client, state = api
    task, path = _make_task(board_repo)
    feature, _ = _make_feature(board_repo)
    feature.tasks[task.task_id] = task
    _ensure_feature(state, "demo", feature)

    with patch("north.service.api.tasks.commit_and_push_board"):
        resp = client.patch(
            "/api/projects/demo/features/feat-x/tasks/001/status",
            json={"status": "blocked", "blocked_reason": "question"},
        )
    assert resp.status_code == 200
    assert feature.tasks["001"].blocked_reason == BlockedReason.QUESTION
    assert parse_task(path).blocked_reason == BlockedReason.QUESTION

    with patch("north.service.api.tasks.commit_and_push_board"):
        resp = client.patch(
            "/api/projects/demo/features/feat-x/tasks/001/status",
            json={"status": "ready"},
        )
    assert resp.status_code == 200
    assert feature.tasks["001"].blocked_reason is None
    assert parse_task(path).blocked_reason is None


def test_patch_to_ready_clears_ready_at(
    api: tuple[TestClient, BoardState], board_repo: Path
) -> None:
    from datetime import UTC, datetime

    from north.service.board.parser import parse_task
    from north.service.board.writer import update_task_frontmatter

    client, state = api
    task, path = _make_task(board_repo, status="in_progress")
    update_task_frontmatter(path, {"ready_at": "2026-06-12T09:00:00+00:00"})
    task.ready_at = datetime(2026, 6, 12, 9, 0, tzinfo=UTC)
    feature, _ = _make_feature(board_repo)
    feature.tasks[task.task_id] = task
    _ensure_feature(state, "demo", feature)

    with patch("north.service.api.tasks.commit_and_push_board"):
        resp = client.patch(
            "/api/projects/demo/features/feat-x/tasks/001/status",
            json={"status": "ready"},
        )
    assert resp.status_code == 200
    assert feature.tasks["001"].ready_at is None
    assert parse_task(path).ready_at is None


def test_patch_blocked_reason_validation(
    api: tuple[TestClient, BoardState], board_repo: Path
) -> None:
    client, state = api
    task, _ = _make_task(board_repo)
    feature, _ = _make_feature(board_repo)
    feature.tasks[task.task_id] = task
    _ensure_feature(state, "demo", feature)

    resp = client.patch(
        "/api/projects/demo/features/feat-x/tasks/001/status",
        json={"status": "done", "blocked_reason": "question"},
    )
    assert resp.status_code == 422

    resp = client.patch(
        "/api/projects/demo/features/feat-x/tasks/001/status",
        json={"status": "blocked", "blocked_reason": "weather"},
    )
    assert resp.status_code == 422


def test_patch_task_done_promotes_feature_to_review(
    api: tuple[TestClient, BoardState], board_repo: Path
) -> None:
    client, state = api
    task, _ = _make_task(board_repo, status="in_progress")
    feature, _ = _make_feature(board_repo)
    feature.tasks[task.task_id] = task
    _ensure_feature(state, "demo", feature)

    with patch("north.service.api.tasks.commit_and_push_board"):
        resp = client.patch(
            "/api/projects/demo/features/feat-x/tasks/001/status",
            json={"status": "done"},
        )

    assert resp.status_code == 200
    assert feature.status == FeatureStatus.REVIEW


def test_patch_task_done_does_not_promote_if_others_pending(
    api: tuple[TestClient, BoardState], board_repo: Path
) -> None:
    client, state = api
    t1, _ = _make_task(board_repo, "001")
    t2, _ = _make_task(board_repo, "002", status="queued")
    feature, _ = _make_feature(board_repo)
    feature.tasks[t1.task_id] = t1
    feature.tasks[t2.task_id] = t2
    _ensure_feature(state, "demo", feature)

    with patch("north.service.api.tasks.commit_and_push_board"):
        client.patch("/api/projects/demo/features/feat-x/tasks/001/status", json={"status": "done"})

    assert feature.status == FeatureStatus.IN_PROGRESS


# --- PATCH /api/features status ----------------------------------------------


def test_patch_feature_status(api: tuple[TestClient, BoardState], board_repo: Path) -> None:
    client, state = api
    feature, _ = _make_feature(board_repo)
    _ensure_feature(state, "demo", feature)

    with patch("north.service.api.features.commit_and_push_board"):
        resp = client.patch("/api/projects/demo/features/feat-x/status", json={"status": "review"})

    assert resp.status_code == 200
    assert state.projects["demo"].features["feat-x"].status == FeatureStatus.REVIEW


def test_patch_feature_404_unknown(api: tuple[TestClient, BoardState]) -> None:
    client, state = api
    _ensure_project(state, "demo")
    with patch("north.service.api.features.commit_and_push_board"):
        resp = client.patch("/api/projects/demo/features/unknown/status", json={"status": "review"})
        assert resp.status_code == 404


# --- POST /api/features requeue ----------------------------------------------


def test_requeue_feature_resets_tasks(
    api: tuple[TestClient, BoardState], board_repo: Path
) -> None:
    client, state = api
    task, _ = _make_task(board_repo, status="done")
    feature, _ = _make_feature(board_repo, status="review")
    feature.tasks[task.task_id] = task
    _ensure_feature(state, "demo", feature)

    with patch("north.service.api.features.commit_and_push_board"):
        resp = client.post("/api/projects/demo/features/feat-x/requeue")

    assert resp.status_code == 200
    assert resp.json()["requeued"] == 1
    updated_feature = state.projects["demo"].features["feat-x"]
    assert updated_feature.tasks["001"].status == TaskStatus.READY
    assert updated_feature.status == FeatureStatus.OPEN


# --- GET /api/projects/{project}/features/{feature}/tasks --------------------


def test_list_tasks_returns_data(api: tuple[TestClient, BoardState], board_repo: Path) -> None:
    client, state = api
    task, _ = _make_task(board_repo)
    feature, _ = _make_feature(board_repo)
    feature.tasks[task.task_id] = task
    _ensure_feature(state, "demo", feature)

    resp = client.get("/api/projects/demo/features/feat-x/tasks")
    assert resp.status_code == 200
    data = resp.json()
    assert isinstance(data, list)
    assert any(t["task_id"] == "001" for t in data)


# --- GET /api/projects/{project}/features/{feature} --------------------------


def test_get_feature_returns_data(api: tuple[TestClient, BoardState], board_repo: Path) -> None:
    client, state = api
    feature, _ = _make_feature(board_repo)
    _ensure_feature(state, "demo", feature)

    resp = client.get("/api/projects/demo/features/feat-x")
    assert resp.status_code == 200
    data = resp.json()
    assert data["feature_id"] == "feat-x"
    assert data["title"] == "Feature feat-x"
    assert data["branch"] == "feat-x"


def test_get_feature_404_unknown(api: tuple[TestClient, BoardState]) -> None:
    client, state = api
    _ensure_project(state, "demo")

    assert client.get("/api/projects/demo/features/unknown").status_code == 404


def test_get_feature_404_unknown_project(api: tuple[TestClient, BoardState]) -> None:
    client, _ = api

    assert client.get("/api/projects/unknown/features/feat-x").status_code == 404


# --- Validation (422) --------------------------------------------------------


def test_patch_task_status_invalid(api: tuple[TestClient, BoardState], board_repo: Path) -> None:
    client, state = api
    task, _ = _make_task(board_repo)
    feature, _ = _make_feature(board_repo)
    feature.tasks[task.task_id] = task
    _ensure_feature(state, "demo", feature)

    resp = client.patch(
        "/api/projects/demo/features/feat-x/tasks/001/status",
        json={"status": "invalid_status"},
    )
    assert resp.status_code == 422


def test_patch_feature_status_invalid(api: tuple[TestClient, BoardState], board_repo: Path) -> None:
    client, state = api
    feature, _ = _make_feature(board_repo)
    _ensure_feature(state, "demo", feature)

    resp = client.patch(
        "/api/projects/demo/features/feat-x/status",
        json={"status": "invalid_status"},
    )
    assert resp.status_code == 422


# --- POST /api/projects/{project}/features ------------------------------------


def test_create_feature_returns_feature_id(
    api: tuple[TestClient, BoardState], board_repo: Path
) -> None:
    client, state = api
    _ensure_project(state, "demo")

    with patch("north.service.api.features.commit_and_push_board"):
        resp = client.post(
            "/api/projects/demo/features",
            json={"title": "New Feature"},
        )

    assert resp.status_code in (200, 201)
    data = resp.json()
    assert data["feature_id"] == "new-feature"
    feature_path = (
        board_repo
        / "projects" / "demo" / "board" / "features" / "active" / "new-feature" / "_feature.md"
    )
    assert feature_path.exists()
    assert "new-feature" in state.projects["demo"].features


def test_create_feature_409_if_exists(api: tuple[TestClient, BoardState], board_repo: Path) -> None:
    client, state = api
    feature, _ = _make_feature(board_repo, feature_id="new-feature")
    _ensure_feature(state, "demo", feature)

    with patch("north.service.api.features.commit_and_push_board"):
        resp = client.post(
            "/api/projects/demo/features",
            json={"title": "New Feature"},
        )

    assert resp.status_code == 409


# --- PUT /api/projects/{project}/features/{feature} ---------------------------


def test_update_feature_updates_title(api: tuple[TestClient, BoardState], board_repo: Path) -> None:
    client, state = api
    feature, _ = _make_feature(board_repo, feature_id="feat-x")
    _ensure_feature(state, "demo", feature)

    with patch("north.service.api.features.commit_and_push_board"):
        resp = client.put(
            "/api/projects/demo/features/feat-x",
            json={"title": "Updated Title", "status": "in_progress", "depends_on": []},
        )

    assert resp.status_code == 200
    assert resp.json()["message"] == "ok"


def test_update_feature_404_unknown(api: tuple[TestClient, BoardState]) -> None:
    client, state = api
    _ensure_project(state, "demo")

    with patch("north.service.api.features.commit_and_push_board"):
        resp = client.put(
            "/api/projects/demo/features/no-such-feature",
            json={"title": "Whatever", "status": "in_progress", "depends_on": []},
        )

    assert resp.status_code == 404


def test_update_feature_status_invalid(
    api: tuple[TestClient, BoardState], board_repo: Path
) -> None:
    client, state = api
    feature, _ = _make_feature(board_repo, feature_id="feat-x")
    _ensure_feature(state, "demo", feature)

    resp = client.put(
        "/api/projects/demo/features/feat-x",
        json={"title": "Whatever", "status": "bad", "depends_on": []},
    )

    assert resp.status_code == 422


# --- POST /api/projects/{project}/features/{feature}/tasks --------------------


def test_create_task_returns_task_id(api: tuple[TestClient, BoardState], board_repo: Path) -> None:
    client, state = api
    feature, _ = _make_feature(board_repo, feature_id="feat-x")
    _ensure_feature(state, "demo", feature)

    with patch("north.service.api.tasks.commit_and_push_board"):
        resp = client.post(
            "/api/projects/demo/features/feat-x/tasks",
            json={"title": "Do something", "pipeline": "default", "depends_on": []},
        )

    assert resp.status_code in (200, 201)
    data = resp.json()
    assert data["task_id"] == "001"
    task_path = (
        board_repo
        / "projects" / "demo" / "board" / "features" / "active" / "feat-x" / "tasks" / "001.md"
    )
    assert task_path.exists()


def test_create_task_increments_id(api: tuple[TestClient, BoardState], board_repo: Path) -> None:
    client, state = api
    feature, _ = _make_feature(board_repo, feature_id="feat-x")
    task, _ = _make_task(board_repo, task_id="001")
    feature.tasks[task.task_id] = task
    _ensure_feature(state, "demo", feature)

    with patch("north.service.api.tasks.commit_and_push_board"):
        resp = client.post(
            "/api/projects/demo/features/feat-x/tasks",
            json={"title": "Second task", "pipeline": "default", "depends_on": []},
        )

    assert resp.status_code in (200, 201)
    assert resp.json()["task_id"] == "002"


def test_create_task_404_no_feature(api: tuple[TestClient, BoardState]) -> None:
    client, state = api
    _ensure_project(state, "demo")

    with patch("north.service.api.tasks.commit_and_push_board"):
        resp = client.post(
            "/api/projects/demo/features/ghost-feature/tasks",
            json={"title": "Task", "pipeline": "default", "depends_on": []},
        )

    assert resp.status_code == 404


# --- PUT /api/projects/{project}/features/{feature}/tasks/{task_id} ----------


def test_update_task_updates_title(api: tuple[TestClient, BoardState], board_repo: Path) -> None:
    client, state = api
    feature, _ = _make_feature(board_repo, feature_id="feat-x")
    task, _ = _make_task(board_repo, task_id="001")
    feature.tasks[task.task_id] = task
    _ensure_feature(state, "demo", feature)

    with patch("north.service.api.tasks.commit_and_push_board"):
        resp = client.put(
            "/api/projects/demo/features/feat-x/tasks/001",
            json={
                "title": "Updated Task",
                "pipeline": "default",
                "status": "queued",
                "depends_on": [],
            },
        )

    assert resp.status_code == 200
    assert resp.json()["message"] == "ok"


def test_update_task_404_unknown(api: tuple[TestClient, BoardState], board_repo: Path) -> None:
    client, state = api
    feature, _ = _make_feature(board_repo, feature_id="feat-x")
    _ensure_feature(state, "demo", feature)

    with patch("north.service.api.tasks.commit_and_push_board"):
        resp = client.put(
            "/api/projects/demo/features/feat-x/tasks/999",
            json={
                "title": "Ghost",
                "pipeline": "default",
                "status": "queued",
                "depends_on": [],
            },
        )

    assert resp.status_code == 404


def test_update_task_status_invalid(api: tuple[TestClient, BoardState], board_repo: Path) -> None:
    client, state = api
    feature, _ = _make_feature(board_repo, feature_id="feat-x")
    task, _ = _make_task(board_repo, task_id="001")
    feature.tasks[task.task_id] = task
    _ensure_feature(state, "demo", feature)

    resp = client.put(
        "/api/projects/demo/features/feat-x/tasks/001",
        json={
            "title": "Task 001",
            "pipeline": "default",
            "status": "bad",
            "depends_on": [],
        },
    )

    assert resp.status_code == 422


# --- DELETE /api/projects/{project}/features/{feature}/tasks/{task_id} -------


def test_delete_task_removes_file(api: tuple[TestClient, BoardState], board_repo: Path) -> None:
    client, state = api
    feature, _ = _make_feature(board_repo, feature_id="feat-x")
    task, path = _make_task(board_repo, task_id="001")
    feature.tasks[task.task_id] = task
    _ensure_feature(state, "demo", feature)

    with patch("north.service.api.tasks.commit_and_push_board") as push:
        resp = client.delete("/api/projects/demo/features/feat-x/tasks/001")

    assert resp.status_code == 200
    assert resp.json()["message"] == "ok"
    assert not path.exists()
    assert "001" not in feature.tasks
    push.assert_called_once()
    assert push.call_args.kwargs["removed"] == [path]


def test_delete_task_404_unknown(api: tuple[TestClient, BoardState], board_repo: Path) -> None:
    client, state = api
    feature, _ = _make_feature(board_repo, feature_id="feat-x")
    _ensure_feature(state, "demo", feature)

    with patch("north.service.api.tasks.commit_and_push_board"):
        resp = client.delete("/api/projects/demo/features/feat-x/tasks/999")

    assert resp.status_code == 404


# --- GET /api/queue (dependency-aware) ----------------------------------------


def _state_task(
    task_id: str,
    status: TaskStatus,
    depends_on: list[str] | None = None,
) -> TaskModel:
    return TaskModel(
        task_id=task_id,
        title=f"task {task_id}",
        status=status,
        pipeline="default",
        task_path=Path(f"/tmp/{task_id}.md"),
        depends_on=depends_on or [],
    )


@pytest.fixture
def queue_api(board_repo: Path) -> tuple[TestClient, BoardState]:
    from north.service import main as main_module

    state = BoardState()
    with patch.object(main_module, "_supervisor", None), patch.object(
        main_module, "_board_state", state
    ):
        yield TestClient(main_module.app), state


def test_queue_excludes_unmet_task_dependency(
    queue_api: tuple[TestClient, BoardState],
) -> None:
    client, state = queue_api
    feature = FeatureModel(
        feature_id="feat-x",
        title="Feature X",
        status=FeatureStatus.IN_PROGRESS,
        feature_path=Path("/tmp/_feature.md"),
    )
    _ensure_feature(state, "demo", feature)
    feature.tasks["001"] = _state_task("001", TaskStatus.QUEUED)
    feature.tasks["002"] = _state_task("002", TaskStatus.QUEUED, depends_on=["001"])

    data = client.get("/api/queue").json()
    assert [t["task_id"] for t in data] == ["001"]


def test_queue_orders_by_dag_depth_when_deps_done(
    queue_api: tuple[TestClient, BoardState],
) -> None:
    client, state = queue_api
    feature = FeatureModel(
        feature_id="feat-x",
        title="Feature X",
        status=FeatureStatus.IN_PROGRESS,
        feature_path=Path("/tmp/_feature.md"),
    )
    _ensure_feature(state, "demo", feature)
    feature.tasks["002"] = _state_task("002", TaskStatus.QUEUED, depends_on=["001"])
    feature.tasks["001"] = _state_task("001", TaskStatus.DONE)
    feature.tasks["003"] = _state_task("003", TaskStatus.QUEUED)

    data = client.get("/api/queue").json()
    assert [t["task_id"] for t in data] == ["003", "002"]


def test_queue_includes_in_progress_before_queued(
    queue_api: tuple[TestClient, BoardState],
) -> None:
    client, state = queue_api
    feature = FeatureModel(
        feature_id="feat-x",
        title="Feature X",
        status=FeatureStatus.IN_PROGRESS,
        feature_path=Path("/tmp/_feature.md"),
    )
    _ensure_feature(state, "demo", feature)
    feature.tasks["001"] = _state_task("001", TaskStatus.IN_PROGRESS)
    feature.tasks["002"] = _state_task("002", TaskStatus.QUEUED)

    data = client.get("/api/queue").json()
    assert [(t["task_id"], t["status"]) for t in data] == [
        ("001", "in_progress"),
        ("002", "queued"),
    ]


def test_queue_project_filter(queue_api: tuple[TestClient, BoardState]) -> None:
    client, state = queue_api
    for project in ("demo", "other"):
        feature = FeatureModel(
            feature_id="feat-x",
            title="Feature X",
            status=FeatureStatus.IN_PROGRESS,
            feature_path=Path("/tmp/_feature.md"),
        )
        _ensure_feature(state, project, feature)
        feature.tasks["001"] = _state_task("001", TaskStatus.QUEUED)

    data = client.get("/api/queue", params={"project": "other"}).json()
    assert [(t["project"], t["task_id"]) for t in data] == [("other", "001")]


# --- DELETE /api/projects/{project}/features/{feature} ------------------------


def test_delete_feature_draft_only(
    api: tuple[TestClient, BoardState], board_repo: Path
) -> None:
    client, state = api
    feature, feature_path = _make_feature(board_repo, feature_id="feat-x")
    task, _ = _make_task(board_repo, task_id="001", status="draft")
    feature.tasks[task.task_id] = task
    _ensure_feature(state, "demo", feature)

    with patch("north.service.api.features.commit_and_push_board") as push:
        resp = client.delete("/api/projects/demo/features/feat-x")

    assert resp.status_code == 200
    assert not feature_path.parent.exists()
    assert "feat-x" not in state.projects["demo"].features
    push.assert_called_once()
    assert push.call_args.kwargs["removed"] == [feature_path.parent]


def test_delete_feature_409_with_non_draft_tasks(
    api: tuple[TestClient, BoardState], board_repo: Path
) -> None:
    client, state = api
    feature, feature_path = _make_feature(board_repo, feature_id="feat-x")
    task, _ = _make_task(board_repo, task_id="001", status="queued")
    feature.tasks[task.task_id] = task
    _ensure_feature(state, "demo", feature)

    with patch("north.service.api.features.commit_and_push_board"):
        resp = client.delete("/api/projects/demo/features/feat-x")

    assert resp.status_code == 409
    assert feature_path.exists()
    assert "feat-x" in state.projects["demo"].features


def test_delete_feature_404_unknown(api: tuple[TestClient, BoardState]) -> None:
    client, state = api
    _ensure_project(state, "demo")
    resp = client.delete("/api/projects/demo/features/unknown")
    assert resp.status_code == 404


# --- GET features: archived include -------------------------------------------


def test_list_features_includes_archived_on_request(
    api: tuple[TestClient, BoardState], board_repo: Path
) -> None:
    client, state = api
    feature, _ = _make_feature(board_repo, feature_id="feat-x")
    _ensure_feature(state, "demo", feature)

    archived_dir = (
        board_repo / "projects" / "demo" / "board" / "features" / "archived" / "feat-old"
    )
    archived_dir.mkdir(parents=True)
    (archived_dir / "_feature.md").write_text(
        "---\nid: feat-old\ntitle: Old\nstatus: merged\n---\nDone.\n",
        encoding="utf-8",
    )

    active_only = client.get("/api/projects/demo/features").json()
    assert [f["feature_id"] for f in active_only] == ["feat-x"]

    both = client.get(
        "/api/projects/demo/features", params={"include": "archived"}
    ).json()
    assert sorted(f["feature_id"] for f in both) == ["feat-old", "feat-x"]
    archived = next(f for f in both if f["feature_id"] == "feat-old")
    assert archived["status"] == "merged"


# --- GET /api/features (global) ------------------------------------------------


def test_global_features_lists_all_projects(
    queue_api: tuple[TestClient, BoardState],
) -> None:
    client, state = queue_api
    for project in ("demo", "other"):
        feature = FeatureModel(
            feature_id=f"feat-{project}",
            title=f"Feature {project}",
            status=FeatureStatus.OPEN,
            feature_path=Path("/tmp/_feature.md"),
        )
        _ensure_feature(state, project, feature)

    data = client.get("/api/features").json()
    assert sorted(f["feature_id"] for f in data) == ["feat-demo", "feat-other"]

    filtered = client.get("/api/features", params={"project": "other"}).json()
    assert [f["feature_id"] for f in filtered] == ["feat-other"]


def test_derive_project_name_strips_git_suffix() -> None:
    from north.service.main import derive_project_name

    assert derive_project_name("git@github.com:org/my-repo.git") == "my-repo"
    assert derive_project_name("git@github.com:org/tagit.git") == "tagit"
    assert derive_project_name("git@github.com:org/widget/") == "widget"
