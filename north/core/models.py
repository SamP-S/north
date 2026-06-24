"""Board data model: a single task object and its status.

Statuses are hardcoded for the MVP (configurable statuses are deferred — see
``docs/plans/037_installable-tool.md``). The status of a task is the folder it
lives in; the ``status`` frontmatter key is a synced mirror.
"""

from dataclasses import dataclass, field
from datetime import datetime
from enum import StrEnum
from pathlib import Path
from typing import Any


class TaskStatus(StrEnum):
    DRAFT = "draft"
    READY = "ready"
    IN_PROGRESS = "in_progress"
    DONE = "done"
    FAILED = "failed"
    BLOCKED = "blocked"


# Status folders created by `north init`, in board order.
STATUS_DIRS: tuple[str, ...] = tuple(s.value for s in TaskStatus)

# Legal status transitions. draft→ready is the human gate; failed/blocked/done
# return to ready for rework. Illegal jumps are rejected (Conflict).
TRANSITIONS: dict[TaskStatus, set[TaskStatus]] = {
    TaskStatus.DRAFT: {TaskStatus.READY},
    TaskStatus.READY: {TaskStatus.IN_PROGRESS},
    TaskStatus.IN_PROGRESS: {TaskStatus.DONE, TaskStatus.FAILED, TaskStatus.BLOCKED},
    TaskStatus.DONE: {TaskStatus.READY},
    TaskStatus.FAILED: {TaskStatus.READY},
    TaskStatus.BLOCKED: {TaskStatus.READY},
}


@dataclass
class Task:
    """One board task. ``path`` is where the file currently lives on disk."""

    id: str
    title: str
    status: TaskStatus
    path: Path
    agent: str = ""
    labels: list[str] = field(default_factory=list)
    depends_on: list[str] = field(default_factory=list)
    created_at: datetime | None = None
    updated_at: datetime | None = None
    body: str = ""
    archived: bool = False

    def to_dict(self) -> dict[str, Any]:
        """Stable, JSON-serialisable view used by ``--json`` and MCP tools."""
        return {
            "id": self.id,
            "title": self.title,
            "status": str(self.status),
            "agent": self.agent,
            "labels": list(self.labels),
            "depends_on": list(self.depends_on),
            "created_at": self.created_at.isoformat() if self.created_at else None,
            "updated_at": self.updated_at.isoformat() if self.updated_at else None,
            "archived": self.archived,
            "body": self.body,
        }
