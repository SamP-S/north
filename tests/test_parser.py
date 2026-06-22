"""Tests for the board file parser."""

from pathlib import Path

import pytest

from north.service.board.parser import ParseError, parse_feature, parse_projects_yaml, parse_task
from north.service.models import FeatureStatus, TaskStatus


def _task_path(tmp_path: Path, content: str) -> Path:
    """Write a task file under a canonical board path so project/feature are derivable."""
    p = (
        tmp_path
        / "projects" / "demo"
        / "board" / "features" / "active" / "feat-x"
        / "tasks" / "001.md"
    )
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(content, encoding="utf-8")
    return p


def _feature_path(tmp_path: Path, content: str) -> Path:
    p = (
        tmp_path
        / "projects" / "demo"
        / "board" / "features" / "active" / "feat-x"
        / "_feature.md"
    )
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(content, encoding="utf-8")
    return p


# --- parse_task --------------------------------------------------------------


def test_parse_task_minimal(tmp_path: Path) -> None:
    path = _task_path(
        tmp_path,
        "---\nid: 1\ntitle: My Task\nstatus: ready\npipeline: default\n---\n\nDo the thing.\n",
    )
    task = parse_task(path)
    assert task.task_id == "001"
    assert task.title == "My Task"
    assert task.status == TaskStatus.READY
    assert task.pipeline == "default"
    assert task.body == "Do the thing."


def test_parse_task_id_zero_padded(tmp_path: Path) -> None:
    path = _task_path(
        tmp_path,
        "---\nid: 5\ntitle: T\nstatus: queued\npipeline: p\n---\n",
    )
    assert parse_task(path).task_id == "005"


def test_parse_task_with_depends_on(tmp_path: Path) -> None:
    path = _task_path(
        tmp_path,
        "---\nid: 2\ntitle: T\nstatus: ready\npipeline: p\ndepends_on: [1]\n---\n",
    )
    task = parse_task(path)
    assert task.depends_on == ["001"]


def test_parse_task_missing_required_raises(tmp_path: Path) -> None:
    path = _task_path(tmp_path, "---\nid: 1\nstatus: ready\n---\n")
    with pytest.raises(ParseError):
        parse_task(path)


def test_parse_task_invalid_status_raises(tmp_path: Path) -> None:
    path = _task_path(
        tmp_path,
        "---\nid: 1\ntitle: T\nstatus: unknown_status\npipeline: p\n---\n",
    )
    with pytest.raises(ParseError):
        parse_task(path)


# --- parse_feature -----------------------------------------------------------


def test_parse_feature_minimal(tmp_path: Path) -> None:
    path = _feature_path(
        tmp_path,
        "---\nid: feat-x\ntitle: Feature X\nstatus: open\n---\n\nBody.\n",
    )
    feature = parse_feature(path)
    assert feature.feature_id == "feat-x"
    assert feature.title == "Feature X"
    assert feature.status == FeatureStatus.OPEN
    assert feature.branch == "feat-x"


def test_parse_feature_missing_id_raises(tmp_path: Path) -> None:
    path = _feature_path(
        tmp_path,
        "---\ntitle: Feature X\nstatus: open\n---\n",
    )
    with pytest.raises(ParseError):
        parse_feature(path)


def test_parse_feature_invalid_status_raises(tmp_path: Path) -> None:
    path = _feature_path(
        tmp_path,
        "---\nid: feat-x\ntitle: X\nstatus: bad_status\n---\n",
    )
    with pytest.raises(ParseError):
        parse_feature(path)


def test_parse_feature_depends_on(tmp_path: Path) -> None:
    path = _feature_path(
        tmp_path,
        "---\nid: feat-x\ntitle: X\nstatus: open\ndepends_on: [feat-a]\n---\n",
    )
    assert parse_feature(path).depends_on == ["feat-a"]


# --- parse_projects_yaml -----------------------------------------------------


def test_parse_projects_yaml(tmp_path: Path) -> None:
    path = tmp_path / "projects.yaml"
    path.write_text(
        "schema_version: 1\nprojects:\n  demo:\n    ssh_url: git@x:demo.git\n",
        encoding="utf-8",
    )
    projects = parse_projects_yaml(path)
    assert "demo" in projects
    assert projects["demo"].ssh_url == "git@x:demo.git"


def test_parse_projects_yaml_empty(tmp_path: Path) -> None:
    path = tmp_path / "projects.yaml"
    path.write_text("schema_version: 1\nprojects: {}\n", encoding="utf-8")
    assert parse_projects_yaml(path) == {}


def test_projects_yaml_base_branch_round_trip(tmp_path: Path) -> None:
    from north.service.board.writer import write_projects_yaml
    from north.service.models import ProjectModel

    projects = {
        "demo": ProjectModel(
            name="demo", ssh_url="git@example.com:demo.git", base_branch="develop"
        ),
        "other": ProjectModel(name="other", ssh_url="git@example.com:other.git"),
    }
    write_projects_yaml(tmp_path, projects)

    parsed = parse_projects_yaml(tmp_path / "projects.yaml")
    assert parsed["demo"].base_branch == "develop"
    assert parsed["other"].base_branch == "main"


def test_projects_yaml_parses_auto_merge(tmp_path):
    path = tmp_path / "projects.yaml"
    path.write_text(
        "schema_version: 1\n"
        "projects:\n"
        "  demo:\n"
        "    ssh_url: git@example.com:demo.git\n"
        "    auto_merge: true\n"
        "  other:\n"
        "    ssh_url: git@example.com:other.git\n",
        encoding="utf-8",
    )
    projects = parse_projects_yaml(path)
    assert projects["demo"].auto_merge is True
    assert projects["other"].auto_merge is False
