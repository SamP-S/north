import re
import shutil
from datetime import UTC, datetime
from pathlib import Path
from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException
from pydantic import BaseModel

from north.service.api.deps import get_board_context
from north.service.board.loader import load_archived_features
from north.service.board.parser import parse_feature, parse_task
from north.service.board.writer import (
    commit_and_push_board,
    replace_feature_file,
    update_feature_frontmatter,
    update_task_frontmatter,
    write_feature_file,
)
from north.service.models import BoardState, FeatureModel, FeatureStatus, TaskStatus

router = APIRouter(prefix="/api/projects")

BoardCtx = Annotated[tuple[BoardState, Path], Depends(get_board_context)]

# Legal status moves for PATCH/PUT. Draft has no exits here — the promote
# verb is the only way out; merged/closed resurrection goes through the
# requeue endpoint. Sysadmin escape: direct file writes.
_FEATURE_TRANSITIONS: dict[FeatureStatus, set[FeatureStatus]] = {
    FeatureStatus.DRAFT: set(),
    FeatureStatus.OPEN: {
        FeatureStatus.IN_PROGRESS,
        FeatureStatus.BLOCKED,
        FeatureStatus.CLOSED,
    },
    FeatureStatus.IN_PROGRESS: {
        FeatureStatus.REVIEW,
        FeatureStatus.OPEN,
        FeatureStatus.BLOCKED,
        FeatureStatus.CLOSED,
    },
    FeatureStatus.REVIEW: {
        FeatureStatus.MERGED,
        FeatureStatus.CLOSED,
        FeatureStatus.IN_PROGRESS,
        FeatureStatus.OPEN,
    },
    FeatureStatus.MERGED: set(),
    FeatureStatus.CLOSED: set(),
    FeatureStatus.BLOCKED: {
        FeatureStatus.OPEN,
        FeatureStatus.IN_PROGRESS,
        FeatureStatus.CLOSED,
    },
}


def _check_feature_transition(current: FeatureStatus, new: FeatureStatus) -> None:
    if new != current and new not in _FEATURE_TRANSITIONS[current]:
        raise HTTPException(
            status_code=409,
            detail=f"Illegal transition {current} → {new}",
        )


class FeatureCreate(BaseModel):
    title: str
    description: str = ""
    depends_on: list[str] = []
    decomposed_from: str | None = None


class FeatureUpdate(BaseModel):
    title: str
    description: str = ""
    depends_on: list[str] = []
    status: str


class FeatureStatusUpdate(BaseModel):
    status: str


def _feature_summary(feature: FeatureModel, project: str) -> dict:
    return {
        "feature_id": feature.feature_id,
        "title": feature.title,
        "status": feature.status,
        "project": project,
        "branch": feature.branch,
    }


@router.get("/{project}/features")
def list_features(project: str, ctx: BoardCtx, include: str | None = None) -> list[dict]:
    """List a project's active features; `include=archived` appends archived ones."""
    board_state, board_path = ctx
    project_model = board_state.projects.get(project)
    if project_model is None:
        raise HTTPException(status_code=404, detail="Project not found")
    features = [
        _feature_summary(f, project) for f in project_model.features.values()
    ]
    if include == "archived":
        features += [
            _feature_summary(f, project)
            for f in load_archived_features(board_path, project)
        ]
    return features


@router.get("/{project}/features/{feature}")
def get_feature(project: str, feature: str, ctx: BoardCtx) -> dict:
    board_state, _ = ctx
    project_model = board_state.projects.get(project)
    if project_model is None:
        raise HTTPException(status_code=404, detail="Project not found")
    feature_model = project_model.features.get(feature)
    if feature_model is None:
        raise HTTPException(status_code=404, detail="Feature not found")
    return {
        "feature_id": feature_model.feature_id,
        "title": feature_model.title,
        "status": feature_model.status,
        "project": project,
        "branch": feature_model.branch,
        "description": feature_model.description,
        "depends_on": feature_model.depends_on,
        "created_at": feature_model.created_at.isoformat()
        if feature_model.created_at
        else None,
        "merged_at": feature_model.merged_at.isoformat() if feature_model.merged_at else None,
        "decomposed_from": feature_model.decomposed_from,
    }


@router.post("/{project}/features")
def create_feature(project: str, body: FeatureCreate, ctx: BoardCtx) -> dict:
    board_state, board_path = ctx
    project_model = board_state.projects.get(project)
    if project_model is None:
        raise HTTPException(status_code=404, detail="Project not found")

    feature_id = re.sub(r"[^a-z0-9]+", "-", body.title.lower()).strip("-")
    feature_dir = board_path / "projects" / project / "board" / "features" / "active" / feature_id
    if feature_dir.exists() or feature_id in project_model.features:
        raise HTTPException(status_code=409, detail="Feature already exists")
    meta = {
        "id": feature_id,
        "title": body.title,
        "status": "draft",
        "depends_on": body.depends_on,
        "created_at": datetime.now(UTC).isoformat(),
    }
    if body.decomposed_from:
        meta["decomposed_from"] = body.decomposed_from
    write_feature_file(feature_dir, feature_id, meta, body.description)
    feature_file = feature_dir / "_feature.md"
    commit_and_push_board(
        board_path, f"[board:feature] create {project}/{feature_id}", [feature_dir]
    )
    project_model.features[feature_id] = parse_feature(feature_file)
    return {"message": "ok", "feature_id": feature_id}


@router.post("/{project}/features/{feature}/promote")
def promote_feature(project: str, feature: str, ctx: BoardCtx) -> dict:
    """Promote a draft feature to open (the only exit from draft)."""
    board_state, board_path = ctx
    project_model = board_state.projects.get(project)
    if project_model is None:
        raise HTTPException(status_code=404, detail="Project not found")
    feature_model = project_model.features.get(feature)
    if feature_model is None:
        raise HTTPException(status_code=404, detail="Feature not found")
    if feature_model.status != FeatureStatus.DRAFT:
        raise HTTPException(status_code=409, detail="Only draft features can be promoted")

    update_feature_frontmatter(feature_model.feature_path, {"status": "open"})
    commit_and_push_board(
        board_path,
        f"[board:feature] promote {project}/{feature}",
        [feature_model.feature_path],
    )
    feature_model.status = FeatureStatus.OPEN
    return {"message": "ok", "status": "open"}


@router.put("/{project}/features/{feature}")
def update_feature(project: str, feature: str, body: FeatureUpdate, ctx: BoardCtx) -> dict:
    board_state, board_path = ctx
    try:
        new_status = FeatureStatus(body.status)
    except ValueError:
        raise HTTPException(status_code=422, detail="Invalid status")

    project_model = board_state.projects.get(project)
    if project_model is None:
        raise HTTPException(status_code=404, detail="Project not found")

    active_root = board_path / "projects" / project / "board" / "features" / "active"
    feature_file = active_root / feature / "_feature.md"
    if not feature_file.exists():
        raise HTTPException(status_code=404, detail="Feature not found")
    current_model = project_model.features.get(feature)
    if current_model is not None:
        _check_feature_transition(current_model.status, new_status)
    replace_feature_file(
        feature_file,
        {
            "title": body.title,
            "status": body.status,
            "depends_on": body.depends_on,
        },
        body.description,
    )
    commit_and_push_board(
        board_path, f"[board:feature] update {project}/{feature}", [feature_file]
    )
    project_model.features[feature] = parse_feature(feature_file)
    return {"message": "ok"}


@router.patch("/{project}/features/{feature}/status")
def update_feature_status(
    project: str, feature: str, body: FeatureStatusUpdate, ctx: BoardCtx
) -> dict:
    board_state, board_path = ctx
    try:
        new_status = FeatureStatus(body.status)
    except ValueError:
        raise HTTPException(status_code=422, detail="Invalid status")

    project_model = board_state.projects.get(project)
    if project_model is None:
        raise HTTPException(status_code=404, detail="Project not found")

    active_dir = board_path / "projects" / project / "board" / "features" / "active" / feature
    feature_file = active_dir / "_feature.md"
    if not feature_file.exists():
        raise HTTPException(status_code=404, detail="Feature not found")
    current_model = project_model.features.get(feature)
    if current_model is not None:
        _check_feature_transition(current_model.status, new_status)

    update_feature_frontmatter(feature_file, {"status": body.status})
    paths = [feature_file]
    removed: list[Path] | None = None

    if body.status in ("merged", "closed"):
        features_archived = board_path / "projects" / project / "board" / "features" / "archived"
        archived_dir = features_archived / feature
        archived_dir.parent.mkdir(parents=True, exist_ok=True)
        shutil.move(str(active_dir), str(archived_dir))
        paths = [archived_dir]
        # stage the active-side deletions too, or the board repo stays
        # dirty and remote sync (pull --rebase) fails forever
        removed = [active_dir]

    commit_and_push_board(
        board_path,
        f"[board:feature] {project}/{feature} → {body.status}",
        paths,
        removed=removed,
    )

    if body.status in ("merged", "closed"):
        project_model.features.pop(feature, None)
    elif feature in project_model.features:
        project_model.features[feature].status = FeatureStatus(body.status)

    return {"message": "ok", "status": body.status}


@router.delete("/{project}/features/{feature}")
def delete_feature(project: str, feature: str, ctx: BoardCtx) -> dict:
    """Delete an active feature directory entirely (mistake cleanup).

    Rejected with 409 if the feature has any task beyond draft — use the
    close/archive lifecycle for features with real history.
    """
    board_state, board_path = ctx
    project_model = board_state.projects.get(project)
    if project_model is None:
        raise HTTPException(status_code=404, detail="Project not found")
    feature_model = project_model.features.get(feature)
    feature_dir = board_path / "projects" / project / "board" / "features" / "active" / feature
    if feature_model is None or not feature_dir.is_dir():
        raise HTTPException(status_code=404, detail="Feature not found")

    non_draft = [
        t.task_id
        for t in feature_model.tasks.values()
        if t.status != TaskStatus.DRAFT
    ]
    if non_draft:
        raise HTTPException(
            status_code=409,
            detail=f"Feature has non-draft tasks: {', '.join(sorted(non_draft))}",
        )

    shutil.rmtree(feature_dir)
    commit_and_push_board(
        board_path,
        f"[board:feature] delete {project}/{feature}",
        [],
        removed=[feature_dir],
    )
    del project_model.features[feature]
    return {"message": "ok"}


@router.post("/{project}/features/{feature}/requeue")
def requeue_feature(project: str, feature: str, ctx: BoardCtx) -> dict:
    board_state, board_path = ctx
    project_model = board_state.projects.get(project)
    if project_model is None:
        raise HTTPException(status_code=404, detail="Project not found")

    active_dir = board_path / "projects" / project / "board" / "features" / "active" / feature
    archived_dir = board_path / "projects" / project / "board" / "features" / "archived" / feature

    unarchived = False
    if active_dir.is_dir():
        feature_dir = active_dir
    elif archived_dir.is_dir():
        active_dir.parent.mkdir(parents=True, exist_ok=True)
        shutil.move(str(archived_dir), str(active_dir))
        feature_dir = active_dir
        unarchived = True
    else:
        raise HTTPException(status_code=404, detail="Feature not found")

    tasks_dir = feature_dir / "tasks"
    task_files: list[Path] = []
    if tasks_dir.is_dir():
        task_files = [p for p in tasks_dir.glob("*.md") if not p.name.endswith(".result.md")]

    for task_file in task_files:
        update_task_frontmatter(task_file, {"status": "ready"})

    feature_file = feature_dir / "_feature.md"
    update_feature_frontmatter(feature_file, {"status": "open"})

    commit_and_push_board(
        board_path,
        f"[board:feature] {project}/{feature} → open (requeued)",
        ([feature_dir] if unarchived else task_files + [feature_file]),
        removed=[archived_dir] if unarchived else None,
    )

    try:
        feature_model = parse_feature(feature_file)
    except Exception:
        feature_model = project_model.features.get(feature)
        if feature_model is not None:
            feature_model.status = FeatureStatus.OPEN

    if feature_model is not None:
        project_model.features[feature] = feature_model
        for task_file in task_files:
            try:
                task = parse_task(task_file)
                feature_model.tasks[task.task_id] = task
            except Exception:
                pass

    return {"message": "ok", "requeued": len(task_files)}
