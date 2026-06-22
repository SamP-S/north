"""Tests for startup validation and in-progress task reset."""

import subprocess
from pathlib import Path

from borealis.service.startup import run_startup_validation


def _run(cwd: Path, *args: str) -> str:
    result = subprocess.run(
        ["git", *args], cwd=str(cwd), check=True, capture_output=True, text=True
    )
    return result.stdout.strip()


def _make_board(tmp_path: Path) -> Path:
    repo = tmp_path / "board"
    repo.mkdir()
    _run(repo, "init", "-b", "main")
    _run(repo, "config", "user.email", "test@test.com")
    _run(repo, "config", "user.name", "Test")
    (repo / "projects.yaml").write_text(
        "schema_version: 1\nprojects:\n  demo:\n    ssh_url: git@example.com:demo.git\n",
        encoding="utf-8",
    )

    feature_dir = repo / "projects" / "demo" / "board" / "features" / "active" / "feat-x"
    tasks_dir = feature_dir / "tasks"
    tasks_dir.mkdir(parents=True)
    (feature_dir / "_feature.md").write_text(
        "---\nid: feat-x\ntitle: Feature X\nstatus: in_progress\n---\nbody\n",
        encoding="utf-8",
    )
    (tasks_dir / "001.md").write_text(
        "---\nid: 001\ntitle: Task one\nstatus: in_progress\npipeline: default\n---\nbody\n",
        encoding="utf-8",
    )
    (tasks_dir / "002.md").write_text(
        "---\nid: 002\ntitle: Task two\nstatus: done\npipeline: default\n---\nbody\n",
        encoding="utf-8",
    )
    _run(repo, "add", "-A")
    _run(repo, "commit", "-m", "init board")
    return repo


def test_startup_resets_in_progress_tasks_to_queued(tmp_path: Path) -> None:
    repo = _make_board(tmp_path)
    head_before = _run(repo, "rev-parse", "HEAD")

    run_startup_validation(repo)

    task_file = (
        repo / "projects" / "demo" / "board" / "features" / "active" / "feat-x"
        / "tasks" / "001.md"
    )
    assert "status: queued" in task_file.read_text(encoding="utf-8")

    done_file = task_file.parent / "002.md"
    assert "status: done" in done_file.read_text(encoding="utf-8")

    head_after = _run(repo, "rev-parse", "HEAD")
    assert head_after != head_before
    assert "reset in_progress" in _run(repo, "log", "-1", "--pretty=%s")


def test_startup_no_commit_when_nothing_in_progress(tmp_path: Path) -> None:
    repo = _make_board(tmp_path)
    run_startup_validation(repo)
    head = _run(repo, "rev-parse", "HEAD")

    run_startup_validation(repo)

    assert _run(repo, "rev-parse", "HEAD") == head
