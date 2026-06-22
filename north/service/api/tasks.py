from datetime import UTC, datetime
from pathlib import Path
from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException
from pydantic import BaseModel

from north.service.api.deps import get_board_context
from north.service.board.parser import parse_task
from north.service.board.writer import (
    commit_and_push_board,
    delete_task_files,
    replace_task_file,
    update_feature_frontmatter,
    update_task_frontmatter,
    write_task_file,
)
from north.service.events import emit_event
from north.service.models import BlockedReason, BoardState, FeatureStatus, TaskStatus

router = APIRouter(prefix="/api/projects")

BoardCtx = Annotated[tuple[BoardState, Path], Depends(get_board_context)]

# Legal status moves for PATCH/PUT. Draft has no exits here — the promote
# verb is the only way out (server-enforced draft gate); `superseded` is
# entered only by the split verb. Sysadmin escape: direct file writes.
_TASK_TRANSITIONS: dict[TaskStatus, set[TaskStatus]] = {
    TaskStatus.DRAFT: set(),
    TaskStatus.READY: {TaskStatus.QUEUED, TaskStatus.BLOCKED},
    TaskStatus.QUEUED: {TaskStatus.IN_PROGRESS, TaskStatus.READY, TaskStatus.BLOCKED},
    TaskStatus.IN_PROGRESS: {
        TaskStatus.DONE,
        TaskStatus.FAILED,
        TaskStatus.BLOCKED,
        TaskStatus.READY,
        TaskStatus.QUEUED,
    },
    TaskStatus.DONE: {TaskStatus.READY},
    TaskStatus.FAILED: {TaskStatus.READY},
    TaskStatus.BLOCKED: {TaskStatus.READY},
    TaskStatus.SUPERSEDED: set(),
}


def _check_task_transition(current: TaskStatus, new: TaskStatus) -> None:
    if new != current and new not in _TASK_TRANSITIONS[current]:
        raise HTTPException(
            status_code=409,
            detail=f"Illegal transition {current} → {new}",
        )


class TaskCreate(BaseModel):
    title: str
    pipeline: str
    body: str = ""
    depends_on: list[str] = []
    decomposed_from: str | None = None


class TaskUpdate(BaseModel):
    title: str
    pipeline: str
    body: str = ""
    depends_on: list[str] = []
    status: str


class TaskStatusUpdate(BaseModel):
    status: str
    result_content: str | None = None
    blocked_reason: str | None = None


class SplitTask(BaseModel):
    title: str
    body: str = ""
    pipeline: str | None = None  # defaults to the parent's pipeline


class TaskSplit(BaseModel):
    tasks: list[SplitTask]


@router.post("/{project}/features/{feature}/tasks")
def create_task(project: str, feature: str, body: TaskCreate, ctx: BoardCtx) -> dict:
    board_state, board_path = ctx
    project_model = board_state.projects.get(project)
    if project_model is None:
        raise HTTPException(status_code=404, detail="Project not found")
    feature_model = project_model.features.get(feature)
    if feature_model is None:
        raise HTTPException(status_code=404, detail="Feature not found")

    active_root = board_path / "projects" / project / "board" / "features" / "active"
    tasks_dir = active_root / feature / "tasks"

    existing_ids = list(feature_model.tasks.keys())
    max_num = max((int(t) for t in existing_ids if t.isdigit()), default=0)
    task_id = str(max_num + 1).zfill(3)
    meta = {
        "id": task_id,
        "title": body.title,
        "status": "draft",
        "pipeline": body.pipeline,
        "depends_on": body.depends_on,
        "created_at": datetime.now(UTC).isoformat(),
    }
    if body.decomposed_from:
        meta["decomposed_from"] = body.decomposed_from
    task_path = write_task_file(tasks_dir, task_id, meta, body.body)
    paths = [task_path]
    message = f"[board:task] create {project}/{feature}/{task_id}"
    # refine rule: adding a task to a feature in review reverts it to
    # in_progress (one commit); it returns to review when tasks complete
    refined = feature_model.status == FeatureStatus.REVIEW
    if refined:
        update_feature_frontmatter(
            feature_model.feature_path, {"status": "in_progress"}
        )
        paths.append(feature_model.feature_path)
        message += " (review → in_progress)"
    commit_and_push_board(board_path, message, paths)
    task = parse_task(task_path)
    feature_model.tasks[task.task_id] = task
    if refined:
        feature_model.status = FeatureStatus.IN_PROGRESS
    return {"message": "ok", "task_id": task_id}


@router.put("/{project}/features/{feature}/tasks/{task_id}")
def update_task(project: str, feature: str, task_id: str, body: TaskUpdate, ctx: BoardCtx) -> dict:
    board_state, board_path = ctx
    try:
        new_status = TaskStatus(body.status)
    except ValueError:
        raise HTTPException(status_code=422, detail="Invalid status")

    project_model = board_state.projects.get(project)
    if project_model is None:
        raise HTTPException(status_code=404, detail="Project not found")
    feature_model = project_model.features.get(feature)
    if feature_model is None:
        raise HTTPException(status_code=404, detail="Feature not found")
    task = feature_model.tasks.get(task_id)
    if task is None:
        raise HTTPException(status_code=404, detail="Task not found")
    _check_task_transition(task.status, new_status)

    replace_task_file(
        task.task_path,
        {
            "title": body.title,
            "pipeline": body.pipeline,
            "status": body.status,
            "depends_on": body.depends_on,
        },
        body.body,
    )
    commit_and_push_board(
        board_path, f"[board:task] update {project}/{feature}/{task_id}", [task.task_path]
    )
    feature_model.tasks[task_id] = parse_task(task.task_path)
    return {"message": "ok"}


@router.delete("/{project}/features/{feature}/tasks/{task_id}")
def delete_task(project: str, feature: str, task_id: str, ctx: BoardCtx) -> dict:
    board_state, board_path = ctx
    project_model = board_state.projects.get(project)
    if project_model is None:
        raise HTTPException(status_code=404, detail="Project not found")
    feature_model = project_model.features.get(feature)
    if feature_model is None:
        raise HTTPException(status_code=404, detail="Feature not found")
    task = feature_model.tasks.get(task_id)
    if task is None:
        raise HTTPException(status_code=404, detail="Task not found")

    deleted_paths = delete_task_files(task.task_path)
    commit_and_push_board(
        board_path,
        f"[board:task] delete {project}/{feature}/{task_id}",
        [],
        removed=deleted_paths,
    )
    del feature_model.tasks[task_id]
    return {"message": "ok"}


@router.get("/{project}/features/{feature}/tasks")
def list_tasks(project: str, feature: str, ctx: BoardCtx) -> list[dict]:
    board_state, _ = ctx
    project_model = board_state.projects.get(project)
    if project_model is None:
        raise HTTPException(status_code=404, detail="Project not found")
    feature_model = project_model.features.get(feature)
    if feature_model is None:
        raise HTTPException(status_code=404, detail="Feature not found")

    return [
        {
            "task_id": t.task_id,
            "title": t.title,
            "status": t.status,
            "pipeline": t.pipeline,
            "project": project,
            "feature": feature,
            "task_path": str(t.task_path),
            "depends_on": t.depends_on,
            "created_at": t.created_at.isoformat() if t.created_at else None,
            "ready_at": t.ready_at.isoformat() if t.ready_at else None,
            "blocked_reason": t.blocked_reason,
            "split_from": t.split_from,
            "decomposed_from": t.decomposed_from,
        }
        for t in feature_model.tasks.values()
    ]


@router.get("/{project}/features/{feature}/tasks/{task_id}")
def get_task(project: str, feature: str, task_id: str, ctx: BoardCtx) -> dict:
    board_state, _ = ctx
    project_model = board_state.projects.get(project)
    if project_model is None:
        raise HTTPException(status_code=404, detail="Project not found")
    feature_model = project_model.features.get(feature)
    if feature_model is None:
        raise HTTPException(status_code=404, detail="Feature not found")
    task = feature_model.tasks.get(task_id)
    if task is None:
        raise HTTPException(status_code=404, detail="Task not found")

    result_path = task.task_path.parent / f"{task.task_path.stem}.result.md"
    result_content = result_path.read_text(encoding="utf-8") if result_path.exists() else None
    return {
        "task_id": task.task_id,
        "title": task.title,
        "status": task.status,
        "pipeline": task.pipeline,
        "project": project,
        "feature": feature,
        "task_path": str(task.task_path),
        "depends_on": task.depends_on,
        "created_at": task.created_at.isoformat() if task.created_at else None,
        "ready_at": task.ready_at.isoformat() if task.ready_at else None,
        "blocked_reason": task.blocked_reason,
        "split_from": task.split_from,
        "decomposed_from": task.decomposed_from,
        "body": task.body,
        "result_content": result_content,
    }


_UNSPLITTABLE = {TaskStatus.DONE, TaskStatus.SUPERSEDED, TaskStatus.IN_PROGRESS}


@router.post("/{project}/features/{feature}/tasks/{task_id}/split")
def split_task(
    project: str, feature: str, task_id: str, body: TaskSplit, ctx: BoardCtx
) -> dict:
    """Replace a task with children, relinking the dependency graph atomically.

    Children inherit the parent's `depends_on` and carry `split_from`;
    dependents of the parent are re-pointed to all children; the parent
    becomes `superseded` (kept for audit). One board commit.
    """
    board_state, board_path = ctx
    if not body.tasks:
        raise HTTPException(status_code=422, detail="Split needs at least one task")
    project_model = board_state.projects.get(project)
    if project_model is None:
        raise HTTPException(status_code=404, detail="Project not found")
    feature_model = project_model.features.get(feature)
    if feature_model is None:
        raise HTTPException(status_code=404, detail="Feature not found")
    parent = feature_model.tasks.get(task_id)
    if parent is None:
        raise HTTPException(status_code=404, detail="Task not found")
    if parent.status in _UNSPLITTABLE:
        raise HTTPException(
            status_code=409, detail=f"Cannot split a task in status {parent.status}"
        )

    tasks_dir = parent.task_path.parent
    existing_ids = list(feature_model.tasks.keys())
    max_num = max((int(t) for t in existing_ids if t.isdigit()), default=0)
    child_ids = [str(max_num + 1 + i).zfill(3) for i in range(len(body.tasks))]
    # children of a promoted parent have already passed the draft gate
    child_status = "draft" if parent.status == TaskStatus.DRAFT else "ready"

    paths: list[Path] = []
    created_at = datetime.now(UTC).isoformat()
    for child_id, child in zip(child_ids, body.tasks):
        meta = {
            "id": child_id,
            "title": child.title,
            "status": child_status,
            "pipeline": child.pipeline or parent.pipeline,
            "depends_on": list(parent.depends_on),
            "created_at": created_at,
            "split_from": task_id,
        }
        paths.append(write_task_file(tasks_dir, child_id, meta, child.body))

    # re-point dependents of the parent to all children
    repointed: list[str] = []
    for sibling in feature_model.tasks.values():
        if task_id not in sibling.depends_on:
            continue
        new_deps = [d for d in sibling.depends_on if d != task_id] + child_ids
        update_task_frontmatter(sibling.task_path, {"depends_on": new_deps})
        paths.append(sibling.task_path)
        repointed.append(sibling.task_id)

    update_task_frontmatter(
        parent.task_path, {"status": "superseded", "blocked_reason": None}
    )
    paths.append(parent.task_path)

    commit_and_push_board(
        board_path,
        f"[board:task] split {project}/{feature}/{task_id} → {', '.join(child_ids)}",
        paths,
    )

    for child_id in child_ids:
        feature_model.tasks[child_id] = parse_task(tasks_dir / f"{child_id}.md")
    for sibling_id in repointed:
        sibling = feature_model.tasks[sibling_id]
        feature_model.tasks[sibling_id] = parse_task(sibling.task_path)
    parent.status = TaskStatus.SUPERSEDED
    parent.blocked_reason = None
    return {"message": "ok", "created": child_ids, "superseded": task_id}


@router.post("/{project}/features/{feature}/tasks/{task_id}/promote")
def promote_task(project: str, feature: str, task_id: str, ctx: BoardCtx) -> dict:
    """Promote a draft task to ready (the only exit from draft)."""
    board_state, board_path = ctx
    project_model = board_state.projects.get(project)
    if project_model is None:
        raise HTTPException(status_code=404, detail="Project not found")
    feature_model = project_model.features.get(feature)
    if feature_model is None:
        raise HTTPException(status_code=404, detail="Feature not found")
    task = feature_model.tasks.get(task_id)
    if task is None:
        raise HTTPException(status_code=404, detail="Task not found")
    if task.status != TaskStatus.DRAFT:
        raise HTTPException(status_code=409, detail="Only draft tasks can be promoted")
    if feature_model.status == FeatureStatus.DRAFT:
        raise HTTPException(
            status_code=409, detail="Promote the feature before its tasks"
        )

    update_task_frontmatter(task.task_path, {"status": "ready"})
    commit_and_push_board(
        board_path,
        f"[board:task] promote {project}/{feature}/{task_id}",
        [task.task_path],
    )
    task.status = TaskStatus.READY
    return {"message": "ok", "status": "ready"}


@router.patch("/{project}/features/{feature}/tasks/{task_id}/status")
def update_task_status(
    project: str, feature: str, task_id: str, body: TaskStatusUpdate, ctx: BoardCtx
) -> dict:
    board_state, board_path = ctx
    try:
        new_status = TaskStatus(body.status)
    except ValueError:
        raise HTTPException(status_code=422, detail="Invalid status")
    blocked_reason: BlockedReason | None = None
    if body.blocked_reason is not None:
        if new_status != TaskStatus.BLOCKED:
            raise HTTPException(
                status_code=422, detail="blocked_reason requires status=blocked"
            )
        try:
            blocked_reason = BlockedReason(body.blocked_reason)
        except ValueError:
            raise HTTPException(status_code=422, detail="Invalid blocked_reason")

    project_model = board_state.projects.get(project)
    if project_model is None:
        raise HTTPException(status_code=404, detail="Project not found")
    feature_model = project_model.features.get(feature)
    if feature_model is None:
        raise HTTPException(status_code=404, detail="Feature not found")
    task = feature_model.tasks.get(task_id)
    if task is None:
        raise HTTPException(status_code=404, detail="Task not found")
    _check_task_transition(task.status, new_status)

    updates: dict = {"status": body.status}
    if new_status == TaskStatus.READY:
        # fresh cooldown on every (re)entry to ready
        updates["ready_at"] = None
    if new_status == TaskStatus.BLOCKED:
        updates["blocked_reason"] = str(blocked_reason) if blocked_reason else None
    elif task.blocked_reason is not None:
        # leaving blocked clears the reason
        updates["blocked_reason"] = None
    update_task_frontmatter(task.task_path, updates)
    paths = [task.task_path]

    if body.result_content is not None:
        result_path = task.task_path.parent / f"{task.task_path.stem}.result.md"
        result_path.write_text(body.result_content, encoding="utf-8")
        paths.append(result_path)

    commit_and_push_board(board_path, f"[system:task] {task_id} → {body.status}", paths)
    task.status = new_status
    task.blocked_reason = blocked_reason if new_status == TaskStatus.BLOCKED else None
    if new_status == TaskStatus.READY:
        task.ready_at = None
    if new_status == TaskStatus.BLOCKED and blocked_reason == BlockedReason.QUESTION:
        emit_event(
            "task_blocked_on_question", project=project, feature=feature, task=task_id
        )
    if new_status == TaskStatus.FAILED:
        emit_event("task_failed", project=project, feature=feature, task=task_id)

    if body.status == TaskStatus.DONE:
        # superseded tasks are audit residue of splits, not open work
        all_done = all(
            t.status in (TaskStatus.DONE, TaskStatus.SUPERSEDED)
            for t in feature_model.tasks.values()
        )
        if all_done:
            feature_file = feature_model.feature_path
            update_feature_frontmatter(feature_file, {"status": "review"})
            commit_and_push_board(
                board_path,
                f"[board:feature] {project}/{feature} → review",
                [feature_file],
            )
            feature_model.status = FeatureStatus.REVIEW

    return {"message": "ok", "status": body.status}
