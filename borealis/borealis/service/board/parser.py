import re
from datetime import datetime
from pathlib import Path
from typing import Any, cast

import frontmatter
import yaml

from borealis.service.models import (
    BlockedReason,
    ConversationModel,
    ConversationStatus,
    FeatureModel,
    FeatureStatus,
    ProjectModel,
    TaskModel,
    TaskStatus,
    ThreadEntry,
    ThreadEntryKind,
)


class ParseError(Exception):
    """Raised when a board file cannot be parsed."""

    def __init__(self, path: Path, original: Exception) -> None:
        super().__init__(f"Failed to parse {path}: {original}")
        self.path = path
        self.original = original


def _pad_id(value: object) -> str:
    return str(value).zfill(3)


def _parse_datetime(value: object) -> datetime | None:
    if not value:
        return None
    try:
        return datetime.fromisoformat(str(value))
    except (ValueError, TypeError):
        return None


def parse_task(path: Path) -> TaskModel:
    """Parse a task Markdown file into a TaskModel."""
    try:
        post = frontmatter.load(str(path))
        meta: dict[str, Any] = cast(dict[str, Any], post.metadata)
        depends_on = [_pad_id(dep) for dep in meta.get("depends_on", [])]
        blocked_reason = (
            BlockedReason(str(meta["blocked_reason"])) if meta.get("blocked_reason") else None
        )
        return TaskModel(
            task_id=_pad_id(meta["id"]),
            title=meta["title"],
            status=TaskStatus(meta["status"]),
            pipeline=str(meta.get("pipeline", "")),
            task_path=path,
            depends_on=depends_on,
            created_at=_parse_datetime(meta.get("created_at")),
            ready_at=_parse_datetime(meta.get("ready_at")),
            blocked_reason=blocked_reason,
            split_from=_pad_id(meta["split_from"]) if meta.get("split_from") else None,
            decomposed_from=(
                _pad_id(meta["decomposed_from"]) if meta.get("decomposed_from") else None
            ),
            body=post.content.strip(),
        )
    except Exception as exc:
        raise ParseError(path, exc) from exc


def parse_conversation(path: Path) -> ConversationModel:
    """Parse a conversation Markdown file into a ConversationModel."""
    try:
        post = frontmatter.load(str(path))
        meta: dict[str, Any] = cast(dict[str, Any], post.metadata)
        decomposed_into = [str(ref) for ref in meta.get("decomposed_into", [])]
        return ConversationModel(
            conversation_id=_pad_id(meta["id"]),
            title=meta["title"],
            status=ConversationStatus(meta["status"]),
            conversation_path=path,
            source=str(meta.get("source", "text")),
            created_at=_parse_datetime(meta.get("created_at")),
            decomposed_into=decomposed_into,
            body=post.content.strip(),
        )
    except Exception as exc:
        raise ParseError(path, exc) from exc


def parse_feature(path: Path) -> FeatureModel:
    """Parse a feature `_feature.md` file into a FeatureModel."""
    try:
        post = frontmatter.load(str(path))
        meta: dict[str, Any] = cast(dict[str, Any], post.metadata)
        depends_on = [str(dep) for dep in meta.get("depends_on", [])]
        return FeatureModel(
            feature_id=str(meta["id"]),
            title=meta["title"],
            status=FeatureStatus(meta["status"]),
            feature_path=path,
            description=str(meta.get("description", "")),
            depends_on=depends_on,
            created_at=_parse_datetime(meta.get("created_at")),
            merged_at=_parse_datetime(meta.get("merged_at")),
            decomposed_from=(
                _pad_id(meta["decomposed_from"]) if meta.get("decomposed_from") else None
            ),
        )
    except Exception as exc:
        raise ParseError(path, exc) from exc


_THREAD_HEADER = re.compile(
    r"^## \[(?P<kind>question|answer|note)\] (?P<author>.+?) — (?P<at>\S+)\s*$"
)


def parse_thread(path: Path) -> list[ThreadEntry]:
    """Parse a thread companion file into entries.

    Lenient by design (threads may be hand-edited): sections whose header
    line does not match the entry format are skipped, never fatal. A missing
    file is an empty thread.
    """
    if not path.exists():
        return []
    entries: list[ThreadEntry] = []
    current: ThreadEntry | None = None
    body_lines: list[str] = []

    def _flush() -> None:
        if current is not None:
            current.text = "\n".join(body_lines).strip()
            entries.append(current)

    for line in path.read_text(encoding="utf-8").splitlines():
        match = _THREAD_HEADER.match(line)
        if match:
            _flush()
            at = _parse_datetime(match.group("at"))
            if at is None:
                current = None
                body_lines = []
                continue
            current = ThreadEntry(
                kind=ThreadEntryKind(match.group("kind")),
                author=match.group("author"),
                at=at,
                text="",
            )
            body_lines = []
        elif current is not None:
            body_lines.append(line)
    _flush()
    return entries


def parse_projects_yaml(path: Path) -> dict[str, ProjectModel]:
    """Parse `projects.yaml` into a mapping of project name to ProjectModel."""
    try:
        with open(path, encoding="utf-8") as handle:
            data = yaml.safe_load(handle)
        if not data or "projects" not in data:
            return {}
        projects: dict[str, ProjectModel] = {}
        for name, entry in data["projects"].items():
            projects[name] = ProjectModel(
                name=name,
                ssh_url=entry["ssh_url"],
                base_branch=str(entry.get("base_branch", "main")),
                auto_merge=bool(entry.get("auto_merge", False)),
            )
        return projects
    except Exception as exc:
        raise ParseError(path, exc) from exc
