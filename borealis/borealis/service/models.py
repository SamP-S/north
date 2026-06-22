from dataclasses import dataclass, field
from datetime import datetime
from enum import StrEnum
from pathlib import Path


class TaskStatus(StrEnum):
    DRAFT = "draft"
    READY = "ready"
    QUEUED = "queued"
    IN_PROGRESS = "in_progress"
    DONE = "done"
    FAILED = "failed"
    BLOCKED = "blocked"
    SUPERSEDED = "superseded"


class FeatureStatus(StrEnum):
    DRAFT = "draft"
    OPEN = "open"
    IN_PROGRESS = "in_progress"
    REVIEW = "review"
    MERGED = "merged"
    CLOSED = "closed"
    BLOCKED = "blocked"


class ConversationStatus(StrEnum):
    PENDING = "pending"
    DECOMPOSING = "decomposing"
    DECOMPOSED = "decomposed"


class BlockedReason(StrEnum):
    QUESTION = "question"
    DEPENDENCY = "dependency"
    INFRA = "infra"


class ThreadEntryKind(StrEnum):
    QUESTION = "question"
    ANSWER = "answer"
    NOTE = "note"


@dataclass
class TaskModel:
    task_id: str
    title: str
    status: TaskStatus
    pipeline: str
    task_path: Path
    depends_on: list[str] = field(default_factory=list)
    created_at: datetime | None = None
    ready_at: datetime | None = None
    blocked_reason: BlockedReason | None = None
    split_from: str | None = None
    decomposed_from: str | None = None
    body: str = ""


@dataclass
class FeatureModel:
    feature_id: str
    title: str
    status: FeatureStatus
    feature_path: Path
    description: str = ""
    depends_on: list[str] = field(default_factory=list)
    created_at: datetime | None = None
    merged_at: datetime | None = None
    decomposed_from: str | None = None
    tasks: dict[str, TaskModel] = field(default_factory=dict)

    @property
    def branch(self) -> str:
        return self.feature_id


@dataclass
class ConversationModel:
    conversation_id: str
    title: str
    status: ConversationStatus
    conversation_path: Path
    source: str = "text"
    created_at: datetime | None = None
    decomposed_into: list[str] = field(default_factory=list)
    body: str = ""


@dataclass
class ThreadEntry:
    kind: ThreadEntryKind
    author: str
    at: datetime
    text: str


@dataclass
class ProjectModel:
    name: str
    ssh_url: str
    base_branch: str = "main"
    auto_merge: bool = False
    features: dict[str, FeatureModel] = field(default_factory=dict)
    conversations: dict[str, ConversationModel] = field(default_factory=dict)


@dataclass
class BoardState:
    projects: dict[str, ProjectModel] = field(default_factory=dict)
