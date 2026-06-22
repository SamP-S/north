import asyncio
import logging
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel

from borealis.service.api.comments import router as comments_router
from borealis.service.api.conversations import router as conversations_router
from borealis.service.api.deps import board_lock, set_board_context
from borealis.service.api.features import router as features_router
from borealis.service.api.tasks import router as tasks_router
from borealis.service.board.loader import load_board_state
from borealis.service.board.parser import parse_projects_yaml
from borealis.service.board.writer import commit_and_push_board, write_projects_yaml
from borealis.service.config import settings
from borealis.service.logsetup import configure_logging
from borealis.service.mcp import mount_mcp, parse_token_map
from borealis.service.models import (
    BoardState,
    ConversationStatus,
    FeatureStatus,
    ProjectModel,
    TaskModel,
    TaskStatus,
)
from borealis.service.orchestrator.resolver import resolve_eligible_tasks
from borealis.service.orchestrator.supervisor import Supervisor
from borealis.service.startup import run_startup_validation

_logger = logging.getLogger(__name__)

_BOARD_PATH = settings.board_path

_board_state: BoardState = BoardState()
_supervisor: Supervisor | None = None


@asynccontextmanager
async def _lifespan(app: FastAPI) -> AsyncIterator[None]:
    global _board_state, _supervisor
    configure_logging()
    run_startup_validation(_BOARD_PATH)
    _board_state = load_board_state(_BOARD_PATH)
    _supervisor = Supervisor(_BOARD_PATH, initial_state=_board_state)
    set_board_context(lambda: _supervisor.board_state, _BOARD_PATH)
    supervisor_task = asyncio.create_task(_supervisor.run())
    async with _mcp_session_managers():
        yield
    supervisor_task.cancel()


app = FastAPI(title="Borealis", lifespan=_lifespan)
app.include_router(tasks_router)
app.include_router(features_router)
app.include_router(conversations_router)
app.include_router(comments_router)
_mcp_session_managers = mount_mcp(app, token_map=parse_token_map(settings.mcp_tokens))


def _current_board_state() -> BoardState:
    if _supervisor is not None:
        return _supervisor.board_state
    return _board_state


@app.get("/api/health")
def health() -> dict:
    return {"status": "ok"}


@app.get("/api/status")
def status() -> dict:
    board_state = _current_board_state()
    return {
        "runner_state": "running",
        "board_loaded": bool(board_state.projects),
    }


@app.get("/api/projects")
def list_projects() -> list[dict]:
    board_state = _current_board_state()
    return [
        {"name": p.name, "ssh_url": p.ssh_url, "base_branch": p.base_branch}
        for p in board_state.projects.values()
    ]


@app.get("/api/features")
def list_all_features(project: str | None = None) -> list[dict]:
    """List active features across all projects, optionally filtered by project."""
    board_state = _current_board_state()
    return [
        {
            "feature_id": feat.feature_id,
            "title": feat.title,
            "status": feat.status,
            "project": proj.name,
            "branch": feat.branch,
        }
        for proj in board_state.projects.values()
        if project is None or proj.name == project
        for feat in proj.features.values()
    ]


def _queue_entry(project_name: str, feature_id: str, task: TaskModel) -> dict:
    return {
        "task_id": task.task_id,
        "title": task.title,
        "status": task.status,
        "project": project_name,
        "feature": feature_id,
        "ready_at": task.ready_at.isoformat() if task.ready_at else None,
        "pipeline": task.pipeline,
        "body": task.body,
    }


@app.get("/api/queue")
def get_queue(project: str | None = None) -> list[dict]:
    """Return in-progress tasks followed by queued tasks in eligible order.

    Queued tasks come from the resolver, so tasks with unmet task or feature
    dependencies are excluded.
    """
    board_state = _current_board_state()

    in_progress: list[tuple[object, dict]] = []
    for proj in board_state.projects.values():
        if project is not None and proj.name != project:
            continue
        for feat in proj.features.values():
            for task in feat.tasks.values():
                if task.status == TaskStatus.IN_PROGRESS:
                    in_progress.append(
                        (task.ready_at, _queue_entry(proj.name, feat.feature_id, task))
                    )
    in_progress.sort(key=lambda item: (item[0] is None, item[0]))

    queued = [
        _queue_entry(eligible.project.name, eligible.feature.feature_id, eligible.task)
        for eligible in resolve_eligible_tasks(board_state)
        if project is None or eligible.project.name == project
    ]

    return [entry for _, entry in in_progress] + queued


@app.get("/api/conversations/pending")
def get_pending_conversations(project: str | None = None) -> list[dict]:
    """Return pending conversations across projects, oldest first (decomposition queue)."""
    board_state = _current_board_state()
    pending = [
        {
            "conversation_id": conv.conversation_id,
            "title": conv.title,
            "status": conv.status,
            "project": proj.name,
            "source": conv.source,
            "created_at": conv.created_at.isoformat() if conv.created_at else None,
        }
        for proj in board_state.projects.values()
        if project is None or proj.name == project
        for conv in proj.conversations.values()
        if conv.status == ConversationStatus.PENDING
    ]
    pending.sort(key=lambda entry: (entry["created_at"] is None, entry["created_at"]))
    return pending


@app.get("/api/review")
def get_review(project: str | None = None) -> list[dict]:
    board_state = _current_board_state()
    reviews: list[dict] = []
    for proj in board_state.projects.values():
        if project is not None and proj.name != project:
            continue
        for feat in proj.features.values():
            if feat.status == FeatureStatus.REVIEW:
                reviews.append(
                    {
                        "feature_id": feat.feature_id,
                        "title": feat.title,
                        "status": feat.status,
                        "project": proj.name,
                        "branch": feat.branch,
                        "auto_merge": proj.auto_merge,
                    }
                )
    return reviews


class ProjectRegisterBody(BaseModel):
    name: str | None = None
    ssh_url: str
    base_branch: str = "main"
    auto_merge: bool = False


def derive_project_name(ssh_url: str) -> str:
    """Derive a project name from the last path segment of an SSH URL."""
    parts = ssh_url.rstrip("/").removesuffix(".git").split("/")
    return parts[-1] if parts else ssh_url


@app.post("/api/projects/register")
def register_project(body: ProjectRegisterBody) -> dict:
    name = body.name or derive_project_name(body.ssh_url)
    projects_yaml = _BOARD_PATH / "projects.yaml"
    with board_lock:
        projects = parse_projects_yaml(projects_yaml) if projects_yaml.exists() else {}
        projects[name] = ProjectModel(
            name=name,
            ssh_url=body.ssh_url,
            base_branch=body.base_branch,
            auto_merge=body.auto_merge,
        )
        write_projects_yaml(_BOARD_PATH, projects)
        commit_and_push_board(
            _BOARD_PATH,
            f"[board:project] register {name}",
            [projects_yaml],
        )
        _current_board_state().projects[name] = projects[name]
    return {"message": "ok", "name": name}


@app.get("/api/projects/{project}")
def get_project(project: str) -> dict:
    proj = _current_board_state().projects.get(project)
    if proj is None:
        raise HTTPException(status_code=404, detail="Project not found")
    return {
        "name": proj.name,
        "ssh_url": proj.ssh_url,
        "base_branch": proj.base_branch,
        "auto_merge": proj.auto_merge,
    }


class ProjectUpdateBody(BaseModel):
    base_branch: str | None = None
    auto_merge: bool | None = None


@app.patch("/api/projects/{project}")
def update_project(project: str, body: ProjectUpdateBody) -> dict:
    if body.base_branch is None and body.auto_merge is None:
        raise HTTPException(status_code=400, detail="No fields to update")
    projects_yaml = _BOARD_PATH / "projects.yaml"
    with board_lock:
        projects = parse_projects_yaml(projects_yaml) if projects_yaml.exists() else {}
        existing = projects.get(project)
        if existing is None:
            raise HTTPException(status_code=404, detail="Project not found")
        updated = ProjectModel(
            name=existing.name,
            ssh_url=existing.ssh_url,
            base_branch=body.base_branch if body.base_branch is not None else existing.base_branch,
            auto_merge=body.auto_merge if body.auto_merge is not None else existing.auto_merge,
        )
        projects[project] = updated
        write_projects_yaml(_BOARD_PATH, projects)
        commit_and_push_board(
            _BOARD_PATH,
            f"[board:project] update {project}",
            [projects_yaml],
        )
        _current_board_state().projects[project] = updated
    return {
        "name": updated.name,
        "ssh_url": updated.ssh_url,
        "base_branch": updated.base_branch,
        "auto_merge": updated.auto_merge,
    }


@app.delete("/api/projects/{project}")
def unregister_project(project: str) -> dict:
    projects_yaml = _BOARD_PATH / "projects.yaml"
    with board_lock:
        projects = parse_projects_yaml(projects_yaml) if projects_yaml.exists() else {}
        if project not in projects:
            raise HTTPException(status_code=404, detail="Project not found")
        del projects[project]
        write_projects_yaml(_BOARD_PATH, projects)
        commit_and_push_board(
            _BOARD_PATH,
            f"[board:project] unregister {project}",
            [projects_yaml],
        )
        _current_board_state().projects.pop(project, None)
    return {"message": "ok"}
