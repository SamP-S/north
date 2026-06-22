"""Tests for board remote sync and change detection."""

import subprocess
from pathlib import Path

import git

from north.service.orchestrator.git_watcher import detect_git_changes, sync_remote


def _run(cwd: Path, *args: str) -> str:
    result = subprocess.run(
        ["git", *args], cwd=str(cwd), check=True, capture_output=True, text=True
    )
    return result.stdout.strip()


def _make_remote_and_clones(tmp_path: Path) -> tuple[Path, Path]:
    origin = tmp_path / "origin.git"
    origin.mkdir()
    _run(origin, "init", "--bare", "-b", "main")

    seed = tmp_path / "seed"
    _run(tmp_path, "clone", str(origin), str(seed))
    _run(seed, "config", "user.email", "test@test.com")
    _run(seed, "config", "user.name", "Test")
    (seed / "projects.yaml").write_text("schema_version: 1\nprojects: {}\n")
    _run(seed, "add", "-A")
    _run(seed, "commit", "-m", "init")
    _run(seed, "push", "origin", "main")

    local = tmp_path / "local"
    _run(tmp_path, "clone", str(origin), str(local))
    _run(local, "config", "user.email", "test@test.com")
    _run(local, "config", "user.name", "Test")
    return seed, local


def test_sync_remote_pulls_remote_commits(tmp_path: Path) -> None:
    seed, local = _make_remote_and_clones(tmp_path)

    (seed / "note.md").write_text("remote edit\n")
    _run(seed, "add", "-A")
    _run(seed, "commit", "-m", "remote edit")
    _run(seed, "push", "origin", "main")

    repo = git.Repo(local)
    before = repo.head.commit.hexsha
    sync_remote(repo)
    assert repo.head.commit.hexsha != before
    assert (local / "note.md").exists()


def test_sync_remote_noop_without_origin(tmp_path: Path) -> None:
    repo_path = tmp_path / "solo"
    repo_path.mkdir()
    _run(repo_path, "init", "-b", "main")
    _run(repo_path, "config", "user.email", "test@test.com")
    _run(repo_path, "config", "user.name", "Test")
    (repo_path / "a").write_text("a")
    _run(repo_path, "add", "-A")
    _run(repo_path, "commit", "-m", "init")

    repo = git.Repo(repo_path)
    head = repo.head.commit.hexsha
    sync_remote(repo)
    assert repo.head.commit.hexsha == head


def test_detect_git_changes_reloads_after_remote_sync(tmp_path: Path) -> None:
    seed, local = _make_remote_and_clones(tmp_path)
    repo = git.Repo(local)
    last_head = repo.head.commit.hexsha

    (seed / "projects.yaml").write_text(
        "schema_version: 1\nprojects:\n  demo:\n    ssh_url: git@example.com:demo.git\n"
    )
    _run(seed, "add", "-A")
    _run(seed, "commit", "-m", "register demo")
    _run(seed, "push", "origin", "main")

    sync_remote(repo)
    from north.service.models import BoardState

    state, new_head = detect_git_changes(repo, local, BoardState(), last_head)
    assert new_head != last_head
    assert "demo" in state.projects
