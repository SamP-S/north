"""Tests for the task dependency resolver and ready-task promoter."""

import subprocess
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest
from borealis.service.config import settings
from borealis.service.models import (
    BoardState,
    FeatureModel,
    FeatureStatus,
    ProjectModel,
    TaskModel,
    TaskStatus,
)
from borealis.service.orchestrator.resolver import promote_ready_tasks, resolve_eligible_tasks


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


def _task(
    task_id: str,
    status: TaskStatus,
    *,
    depends_on: list[str] | None = None,
    feature: str = "feat-x",
    ready_at: datetime | None = None,
    path: Path | None = None,
) -> TaskModel:
    return TaskModel(
        task_id=task_id,
        title=f"Task {task_id}",
        status=status,
        pipeline="default",
        task_path=path
        or Path(f"/tmp/projects/demo/board/features/active/{feature}/tasks/{task_id}.md"),
        depends_on=depends_on or [],
        ready_at=ready_at,
    )


def _feature(
    feature_id: str = "feat-x",
    status: FeatureStatus = FeatureStatus.IN_PROGRESS,
    *,
    depends_on: list[str] | None = None,
) -> FeatureModel:
    return FeatureModel(
        feature_id=feature_id,
        title=feature_id,
        status=status,
        feature_path=Path(f"/tmp/{feature_id}/_feature.md"),
        depends_on=depends_on or [],
    )


def _state(
    *tasks: TaskModel,
    task_feature: str = "feat-x",
    features: list[FeatureModel] | None = None,
) -> BoardState:
    state = BoardState()
    project = ProjectModel(name="demo", ssh_url="git@x:demo.git")
    state.projects["demo"] = project

    feature_map = {f.feature_id: f for f in (features or [])}
    if task_feature not in feature_map:
        feature_map[task_feature] = _feature(task_feature)

    for t in tasks:
        feature_map[task_feature].tasks[t.task_id] = t

    for f in feature_map.values():
        project.features[f.feature_id] = f

    return state


# --- resolve_eligible_tasks --------------------------------------------------


def test_no_queued_tasks_returns_empty() -> None:
    state = _state(_task("001", TaskStatus.READY))
    assert resolve_eligible_tasks(state) == []


def test_queued_no_deps_eligible() -> None:
    state = _state(_task("001", TaskStatus.QUEUED))
    result = resolve_eligible_tasks(state)
    assert len(result) == 1
    assert result[0].task.task_id == "001"


def test_blocked_by_undone_dependency() -> None:
    t1 = _task("001", TaskStatus.QUEUED)
    t2 = _task("002", TaskStatus.QUEUED, depends_on=["001"])
    state = _state(t1, t2)
    ids = [e.task.task_id for e in resolve_eligible_tasks(state)]
    assert "001" in ids
    assert "002" not in ids


def test_unblocked_when_dependency_done() -> None:
    t1 = _task("001", TaskStatus.DONE)
    t2 = _task("002", TaskStatus.QUEUED, depends_on=["001"])
    state = _state(t1, t2)
    ids = [e.task.task_id for e in resolve_eligible_tasks(state)]
    assert "002" in ids


def test_dag_depth_ordering() -> None:
    now = datetime.now(UTC)
    t1 = _task("001", TaskStatus.DONE)
    t2 = _task("002", TaskStatus.QUEUED, depends_on=["001"], ready_at=now)
    t3 = _task("003", TaskStatus.QUEUED, ready_at=now)
    state = _state(t1, t2, t3)
    result = resolve_eligible_tasks(state)
    # t3 (depth 0) before t2 (depth 1)
    ids = [e.task.task_id for e in result]
    assert ids.index("003") < ids.index("002")


def test_feature_dep_blocks_tasks() -> None:
    t1 = _task("001", TaskStatus.QUEUED, feature="feat-b")
    feat_b = _feature("feat-b", FeatureStatus.IN_PROGRESS, depends_on=["feat-a"])
    feat_a = _feature("feat-a", FeatureStatus.IN_PROGRESS)
    state = _state(t1, task_feature="feat-b", features=[feat_a, feat_b])
    assert resolve_eligible_tasks(state) == []


def test_feature_dep_unblocked_when_merged() -> None:
    t1 = _task("001", TaskStatus.QUEUED, feature="feat-b")
    feat_b = _feature("feat-b", FeatureStatus.IN_PROGRESS, depends_on=["feat-a"])
    feat_a = _feature("feat-a", FeatureStatus.MERGED)
    state = _state(t1, task_feature="feat-b", features=[feat_a, feat_b])
    assert len(resolve_eligible_tasks(state)) == 1


# --- promote_ready_tasks -----------------------------------------------------


def _write_task_file(board_repo: Path, task_id: str, status: str) -> Path:
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
        "pipeline: default\n---\n\nBody.\n",
        encoding="utf-8",
    )
    _run(board_repo, "add", str(path.relative_to(board_repo)))
    _run(board_repo, "commit", "-m", f"add task {task_id}")
    return path


def test_promote_stamps_ready_at(board_repo: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(settings, "cooldown_seconds", 300)
    path = _write_task_file(board_repo, "001", "ready")
    task = _task("001", TaskStatus.READY, path=path)
    state = _state(task)
    now = datetime.now(UTC)

    promote_ready_tasks(state, board_repo, now)

    assert task.ready_at == now


def test_promote_queues_after_cooldown(board_repo: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(settings, "cooldown_seconds", 0)
    path = _write_task_file(board_repo, "001", "ready")
    past = datetime.now(UTC) - timedelta(seconds=1)
    task = _task("001", TaskStatus.READY, ready_at=past, path=path)
    state = _state(task)

    result = promote_ready_tasks(state, board_repo, datetime.now(UTC))

    assert task.status == TaskStatus.QUEUED
    assert len(result) == 1


def test_promote_skips_before_cooldown(board_repo: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(settings, "cooldown_seconds", 9999)
    path = _write_task_file(board_repo, "001", "ready")
    task = _task("001", TaskStatus.READY, ready_at=datetime.now(UTC), path=path)
    state = _state(task)

    result = promote_ready_tasks(state, board_repo, datetime.now(UTC))

    assert task.status == TaskStatus.READY
    assert result == []
