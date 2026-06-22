import logging
from pathlib import Path
from typing import Any

import frontmatter
import git
import yaml

from borealis.service.models import ProjectModel

_logger = logging.getLogger(__name__)


class BoardPushConflictError(Exception):
    """Raised when board changes cannot be pushed to the remote."""


def commit_board(
    repo_path: Path,
    message: str,
    paths: list[Path],
    removed: list[Path] | None = None,
) -> str:
    """Stage the given paths (and removals), commit with the message, return the SHA."""
    repo = git.Repo(repo_path)
    root = repo_path.resolve()
    if paths:
        repo.index.add([str(p.resolve().relative_to(root)) for p in paths])
    if removed:
        repo.index.remove(
            [str(p.resolve().relative_to(root)) for p in removed], r=True
        )
    commit = repo.index.commit(message)
    return commit.hexsha


def commit_and_push_board(
    repo_path: Path,
    message: str,
    paths: list[Path],
    removed: list[Path] | None = None,
) -> str:
    """Commit the given paths and push to origin. Push failure is logged but non-fatal."""
    sha = commit_board(repo_path, message, paths, removed=removed)
    try:
        push_board(repo_path)
    except BoardPushConflictError as exc:
        _logger.error("Board push failed after commit %s: %s", sha[:7], exc)
    return sha


def push_board(repo_path: Path) -> None:
    """Push the current branch to origin, rebasing once on non-fast-forward.

    A no-op when the repo has no `origin` remote (e.g. local-only test boards).
    """
    repo = git.Repo(repo_path)
    if not any(remote.name == "origin" for remote in repo.remotes):
        return
    origin = repo.remote("origin")
    try:
        origin.push()
    except git.exc.GitCommandError as exc:
        if "non-fast-forward" not in str(exc) and "fetch first" not in str(exc):
            _logger.error("Board push failed: %s", exc)
            raise BoardPushConflictError(str(exc)) from exc
        try:
            repo.git.pull("--rebase")
        except git.exc.GitCommandError as rebase_exc:
            repo.git.rebase("--abort")
            _logger.error("Board rebase conflict: %s", rebase_exc)
            raise BoardPushConflictError(str(rebase_exc)) from rebase_exc
        try:
            origin.push()
        except git.exc.GitCommandError as retry_exc:
            _logger.error("Board push retry failed: %s", retry_exc)
            raise BoardPushConflictError(str(retry_exc)) from retry_exc


def write_projects_yaml(board_path: Path, projects: dict[str, ProjectModel]) -> None:
    """Write projects.yaml at board_path / 'projects.yaml'."""
    data = {
        "schema_version": 1,
        "projects": {
            name: {
                "ssh_url": project.ssh_url,
                "base_branch": project.base_branch,
                "auto_merge": project.auto_merge,
            }
            for name, project in projects.items()
        },
    }
    dest = board_path / "projects.yaml"
    with open(dest, "w", encoding="utf-8") as handle:
        yaml.dump(data, handle, default_flow_style=False)


def update_task_frontmatter(task_path: Path, updates: dict[str, Any]) -> None:
    """Update frontmatter keys in a task file, preserving the body."""
    _update_frontmatter(task_path, updates)


def update_feature_frontmatter(feature_path: Path, updates: dict[str, Any]) -> None:
    """Update frontmatter keys in a `_feature.md` file, preserving the body."""
    _update_frontmatter(feature_path, updates)


def replace_task_file(task_path: Path, updates: dict[str, Any], body: str) -> None:
    """Update frontmatter keys and replace the body of a task file."""
    _update_frontmatter(task_path, updates, body=body)


def replace_feature_file(feature_path: Path, updates: dict[str, Any], description: str) -> None:
    """Update frontmatter keys and replace the description body of a `_feature.md` file."""
    _update_frontmatter(feature_path, updates, body=description)


def _update_frontmatter(path: Path, updates: dict[str, Any], body: str | None = None) -> None:
    """Update frontmatter keys in a Markdown file, optionally replacing the body.

    A `None` value removes the key (frontmatter stays free of null entries).
    """
    post = frontmatter.load(str(path))
    for key, value in updates.items():
        if value is None:
            post.metadata.pop(key, None)
        else:
            post[key] = value
    if body is not None:
        post.content = body
    with open(path, "w", encoding="utf-8") as handle:
        frontmatter.dump(post, handle)


def write_feature_file(
    feature_dir: Path, feature_id: str, meta: dict[str, Any], description: str
) -> Path:
    """Create feature directory structure and write _feature.md with frontmatter and body."""
    feature_dir.mkdir(parents=True, exist_ok=True)
    (feature_dir / "tasks").mkdir(parents=True, exist_ok=True)
    post = frontmatter.Post(description, **meta)
    feature_file = feature_dir / "_feature.md"
    with open(feature_file, "w", encoding="utf-8") as handle:
        frontmatter.dump(post, handle)
    return feature_file


def write_task_file(tasks_dir: Path, task_id: str, meta: dict[str, Any], body: str) -> Path:
    """Create tasks directory and write a task Markdown file with frontmatter and body."""
    tasks_dir.mkdir(parents=True, exist_ok=True)
    post = frontmatter.Post(body, **meta)
    task_file = tasks_dir / f"{task_id}.md"
    with open(task_file, "w", encoding="utf-8") as handle:
        frontmatter.dump(post, handle)
    return task_file


def write_conversation_file(
    conversations_dir: Path, conversation_id: str, meta: dict[str, Any], body: str
) -> Path:
    """Create the conversations directory and write a conversation Markdown file."""
    conversations_dir.mkdir(parents=True, exist_ok=True)
    post = frontmatter.Post(body, **meta)
    conversation_file = conversations_dir / f"{conversation_id}.md"
    with open(conversation_file, "w", encoding="utf-8") as handle:
        frontmatter.dump(post, handle)
    return conversation_file


def update_conversation_frontmatter(conversation_path: Path, updates: dict[str, Any]) -> None:
    """Update frontmatter keys in a conversation file, preserving the body."""
    _update_frontmatter(conversation_path, updates)


def append_thread_entry(
    thread_path: Path, kind: str, author: str, at: str, text: str
) -> None:
    """Append one typed entry to a thread companion file (append-only, created on demand)."""
    entry = f"## [{kind}] {author} — {at}\n\n{text.strip()}\n"
    with open(thread_path, "a", encoding="utf-8") as handle:
        if thread_path.stat().st_size > 0:
            handle.write("\n")
        handle.write(entry)


def delete_task_files(task_path: Path) -> list[Path]:
    """Delete a task file and its companion result file, returning the deleted paths."""
    deleted: list[Path] = []
    if task_path.exists():
        task_path.unlink()
        deleted.append(task_path)
    result_path = task_path.parent / f"{task_path.stem}.result.md"
    if result_path.exists():
        result_path.unlink()
        deleted.append(result_path)
    return deleted
