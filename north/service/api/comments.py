"""Comment thread endpoints for tasks and features.

Threads are append-only companion files (`<task>.thread.md`,
`_feature.thread.md`): no edit or delete endpoints, and the parser
tolerates hand-edited files.
"""

from datetime import UTC, datetime
from pathlib import Path
from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException
from pydantic import BaseModel

from north.service.api.deps import get_board_context
from north.service.board.parser import parse_thread
from north.service.board.writer import (
    append_thread_entry,
    commit_and_push_board,
    update_task_frontmatter,
)
from north.service.events import emit_event
from north.service.models import (
    BlockedReason,
    BoardState,
    FeatureModel,
    FeatureStatus,
    TaskModel,
    TaskStatus,
    ThreadEntry,
    ThreadEntryKind,
)

# review briefs arrive as feature-thread notes by this author; their landing
# is the single feature_review notification (carries the brief summary line)
BRIEF_AUTHOR = "north/brief"

router = APIRouter(prefix="/api/projects")

BoardCtx = Annotated[tuple[BoardState, Path], Depends(get_board_context)]


class CommentCreate(BaseModel):
    kind: str
    author: str
    text: str


def _entry_dict(entry: ThreadEntry) -> dict:
    return {
        "kind": entry.kind,
        "author": entry.author,
        "at": entry.at.isoformat(),
        "text": entry.text,
    }


def _get_feature(board_state: BoardState, project: str, feature: str) -> FeatureModel:
    project_model = board_state.projects.get(project)
    if project_model is None:
        raise HTTPException(status_code=404, detail="Project not found")
    feature_model = project_model.features.get(feature)
    if feature_model is None:
        raise HTTPException(status_code=404, detail="Feature not found")
    return feature_model


def _get_task(
    board_state: BoardState, project: str, feature: str, task_id: str
) -> TaskModel:
    feature_model = _get_feature(board_state, project, feature)
    task = feature_model.tasks.get(task_id)
    if task is None:
        raise HTTPException(status_code=404, detail="Task not found")
    return task


def task_thread_path(task: TaskModel) -> Path:
    return task.task_path.parent / f"{task.task_path.stem}.thread.md"


def feature_thread_path(feature: FeatureModel) -> Path:
    return feature.feature_path.parent / "_feature.thread.md"


def _parse_kind(kind: str) -> ThreadEntryKind:
    try:
        return ThreadEntryKind(kind)
    except ValueError:
        raise HTTPException(status_code=422, detail="Invalid comment kind")


@router.get("/{project}/features/{feature}/tasks/{task_id}/comments")
def list_task_comments(
    project: str, feature: str, task_id: str, ctx: BoardCtx
) -> list[dict]:
    board_state, _ = ctx
    task = _get_task(board_state, project, feature, task_id)
    return [_entry_dict(e) for e in parse_thread(task_thread_path(task))]


@router.post("/{project}/features/{feature}/tasks/{task_id}/comments")
def add_task_comment(
    project: str, feature: str, task_id: str, body: CommentCreate, ctx: BoardCtx
) -> dict:
    board_state, board_path = ctx
    kind = _parse_kind(body.kind)
    task = _get_task(board_state, project, feature, task_id)
    thread_path = task_thread_path(task)
    at = datetime.now(UTC).isoformat()
    append_thread_entry(thread_path, str(kind), body.author, at, body.text)
    paths = [thread_path]

    # answering a question-blocked task flips it back to ready (one commit)
    unblocked = (
        kind == ThreadEntryKind.ANSWER
        and task.status == TaskStatus.BLOCKED
        and task.blocked_reason == BlockedReason.QUESTION
    )
    message = f"[board:comment] {project}/{feature}/{task_id} [{kind}] by {body.author}"
    if unblocked:
        update_task_frontmatter(
            task.task_path,
            {"status": "ready", "blocked_reason": None, "ready_at": None},
        )
        paths.append(task.task_path)
        message += " (answered → ready)"
    commit_and_push_board(board_path, message, paths)
    if unblocked:
        task.status = TaskStatus.READY
        task.blocked_reason = None
        task.ready_at = None
    return {"message": "ok", "task_status": str(task.status)}


@router.get("/{project}/features/{feature}/comments")
def list_feature_comments(project: str, feature: str, ctx: BoardCtx) -> list[dict]:
    board_state, _ = ctx
    feature_model = _get_feature(board_state, project, feature)
    return [_entry_dict(e) for e in parse_thread(feature_thread_path(feature_model))]


@router.post("/{project}/features/{feature}/comments")
def add_feature_comment(
    project: str, feature: str, body: CommentCreate, ctx: BoardCtx
) -> dict:
    board_state, board_path = ctx
    kind = _parse_kind(body.kind)
    feature_model = _get_feature(board_state, project, feature)
    thread_path = feature_thread_path(feature_model)
    at = datetime.now(UTC).isoformat()
    append_thread_entry(thread_path, str(kind), body.author, at, body.text)
    commit_and_push_board(
        board_path,
        f"[board:comment] {project}/{feature} [{kind}] by {body.author}",
        [thread_path],
    )
    if (
        kind == ThreadEntryKind.NOTE
        and body.author == BRIEF_AUTHOR
        and feature_model.status == FeatureStatus.REVIEW
    ):
        first_line = next(
            (line.strip() for line in body.text.splitlines() if line.strip()), ""
        )
        emit_event(
            "feature_review", project=project, feature=feature, summary=first_line
        )
    return {"message": "ok"}
