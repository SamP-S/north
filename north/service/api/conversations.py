"""Conversation endpoints: ship, list, detail, status transitions.

Conversations are first-class board objects (work intake, not design docs).
They are audit objects: no delete endpoint, and status only moves forward
(pending → decomposing → decomposed).
"""

from datetime import UTC, datetime
from pathlib import Path
from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException
from pydantic import BaseModel

from north.service.api.deps import get_board_context
from north.service.board.parser import parse_conversation
from north.service.board.writer import (
    commit_and_push_board,
    update_conversation_frontmatter,
    write_conversation_file,
)
from north.service.events import emit_event
from north.service.models import (
    BoardState,
    ConversationModel,
    ConversationStatus,
    ProjectModel,
)

router = APIRouter(prefix="/api/projects")

BoardCtx = Annotated[tuple[BoardState, Path], Depends(get_board_context)]

_ALLOWED_TRANSITIONS: dict[ConversationStatus, set[ConversationStatus]] = {
    ConversationStatus.PENDING: {ConversationStatus.DECOMPOSING},
    # decomposing → pending: a failed decompose returns to the queue
    ConversationStatus.DECOMPOSING: {
        ConversationStatus.DECOMPOSED,
        ConversationStatus.PENDING,
    },
    ConversationStatus.DECOMPOSED: set(),
}


class ConversationCreate(BaseModel):
    title: str
    content: str = ""
    source: str = "text"


class ConversationStatusUpdate(BaseModel):
    status: str
    decomposed_into: list[str] | None = None
    result_content: str | None = None


def _get_project(board_state: BoardState, project: str) -> ProjectModel:
    project_model = board_state.projects.get(project)
    if project_model is None:
        raise HTTPException(status_code=404, detail="Project not found")
    return project_model


def _conversation_summary(conversation: ConversationModel, project: str) -> dict:
    return {
        "conversation_id": conversation.conversation_id,
        "title": conversation.title,
        "status": conversation.status,
        "project": project,
        "source": conversation.source,
        "created_at": (
            conversation.created_at.isoformat() if conversation.created_at else None
        ),
        "decomposed_into": conversation.decomposed_into,
    }


@router.post("/{project}/conversations")
def create_conversation(project: str, body: ConversationCreate, ctx: BoardCtx) -> dict:
    """Ship a condensed conversation onto the board (lands `pending`)."""
    board_state, board_path = ctx
    project_model = _get_project(board_state, project)
    if body.source not in ("text", "voice"):
        raise HTTPException(status_code=422, detail="Invalid source")

    conversations_dir = board_path / "projects" / project / "conversations"
    existing_ids = list(project_model.conversations.keys())
    max_num = max((int(c) for c in existing_ids if c.isdigit()), default=0)
    conversation_id = str(max_num + 1).zfill(3)
    meta = {
        "id": conversation_id,
        "title": body.title,
        "status": "pending",
        "source": body.source,
        "created_at": datetime.now(UTC).isoformat(),
    }
    path = write_conversation_file(conversations_dir, conversation_id, meta, body.content)
    commit_and_push_board(
        board_path,
        f"[board:conversation] create {project}/{conversation_id}",
        [path],
    )
    conversation = parse_conversation(path)
    project_model.conversations[conversation_id] = conversation
    emit_event("conversation_shipped", project=project, conversation=conversation_id)
    return {"message": "ok", "conversation_id": conversation_id}


@router.get("/{project}/conversations")
def list_conversations(project: str, ctx: BoardCtx) -> list[dict]:
    board_state, _ = ctx
    project_model = _get_project(board_state, project)
    return [
        _conversation_summary(c, project)
        for c in project_model.conversations.values()
    ]


@router.get("/{project}/conversations/{conversation_id}")
def get_conversation(project: str, conversation_id: str, ctx: BoardCtx) -> dict:
    board_state, _ = ctx
    project_model = _get_project(board_state, project)
    conversation = project_model.conversations.get(conversation_id)
    if conversation is None:
        raise HTTPException(status_code=404, detail="Conversation not found")
    result_path = (
        conversation.conversation_path.parent
        / f"{conversation.conversation_path.stem}.result.md"
    )
    result_content = (
        result_path.read_text(encoding="utf-8") if result_path.exists() else None
    )
    return _conversation_summary(conversation, project) | {
        "body": conversation.body,
        "result_content": result_content,
    }


@router.patch("/{project}/conversations/{conversation_id}/status")
def update_conversation_status(
    project: str, conversation_id: str, body: ConversationStatusUpdate, ctx: BoardCtx
) -> dict:
    board_state, board_path = ctx
    try:
        new_status = ConversationStatus(body.status)
    except ValueError:
        raise HTTPException(status_code=422, detail="Invalid status")

    project_model = _get_project(board_state, project)
    conversation = project_model.conversations.get(conversation_id)
    if conversation is None:
        raise HTTPException(status_code=404, detail="Conversation not found")
    if new_status not in _ALLOWED_TRANSITIONS[conversation.status]:
        raise HTTPException(
            status_code=409,
            detail=f"Illegal transition {conversation.status} → {new_status}",
        )

    updates: dict = {"status": str(new_status)}
    if body.decomposed_into is not None:
        updates["decomposed_into"] = body.decomposed_into
    update_conversation_frontmatter(conversation.conversation_path, updates)
    paths = [conversation.conversation_path]

    if body.result_content is not None:
        result_path = (
            conversation.conversation_path.parent
            / f"{conversation.conversation_path.stem}.result.md"
        )
        result_path.write_text(body.result_content, encoding="utf-8")
        paths.append(result_path)

    commit_and_push_board(
        board_path,
        f"[board:conversation] {project}/{conversation_id} → {new_status}",
        paths,
    )
    project_model.conversations[conversation_id] = parse_conversation(
        conversation.conversation_path
    )
    if new_status == ConversationStatus.DECOMPOSED:
        created = ", ".join(body.decomposed_into or []) or "nothing"
        emit_event(
            "conversation_decomposed",
            project=project,
            conversation=conversation_id,
            created=created,
            summary=(
                f"{project} conversation {conversation_id} decomposed — "
                f"drafts await promotion: {created}"
            ),
        )
    return {"message": "ok", "status": str(new_status)}
