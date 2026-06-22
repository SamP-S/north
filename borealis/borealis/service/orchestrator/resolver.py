from datetime import datetime, timedelta
from pathlib import Path
from typing import NamedTuple

from borealis.service.board.writer import commit_board, update_task_frontmatter
from borealis.service.config import settings
from borealis.service.models import (
    BoardState,
    FeatureModel,
    FeatureStatus,
    ProjectModel,
    TaskModel,
    TaskStatus,
)


def promote_ready_tasks(
    board_state: BoardState, board_path: Path, now: datetime
) -> list[TaskModel]:
    """Stamp `ready_at` on ready tasks and queue those past their cooldown."""
    queued: list[TaskModel] = []
    cooldown = timedelta(seconds=settings.cooldown_seconds)
    for project in board_state.projects.values():
        for feature in project.features.values():
            for task in feature.tasks.values():
                if task.status != TaskStatus.READY:
                    continue
                if task.ready_at is None:
                    update_task_frontmatter(
                        task.task_path, {"ready_at": now.isoformat()}
                    )
                    commit_board(
                        board_path,
                        f"[system:task] set ready_at for {task.task_id}",
                        [task.task_path],
                    )
                    task.ready_at = now
                if now >= task.ready_at + cooldown:
                    update_task_frontmatter(task.task_path, {"status": "queued"})
                    commit_board(
                        board_path,
                        f"[system:task] queued {task.task_id}",
                        [task.task_path],
                    )
                    task.status = TaskStatus.QUEUED
                    queued.append(task)
    return queued


class EligibleTask(NamedTuple):
    """A queued task that is ready to run, with its owning project and feature."""

    project: ProjectModel
    feature: FeatureModel
    task: TaskModel


def _dag_depth(task: TaskModel, tasks_by_id: dict[str, TaskModel]) -> int:
    memo: dict[str, int] = {}

    def _depth(current: TaskModel) -> int:
        if current.task_id in memo:
            return memo[current.task_id]
        if not current.depends_on:
            memo[current.task_id] = 0
            return 0
        best = 0
        for dep_id in current.depends_on:
            dep = tasks_by_id.get(dep_id)
            if dep is None:
                continue
            best = max(best, 1 + _depth(dep))
        memo[current.task_id] = best
        return best

    return _depth(task)


def _deps_done(task: TaskModel, tasks_by_id: dict[str, TaskModel]) -> bool:
    for dep_id in task.depends_on:
        dep = tasks_by_id.get(dep_id)
        if dep is None or dep.status != TaskStatus.DONE:
            return False
    return True


def _feature_deps_merged(
    feature: FeatureModel,
    project: ProjectModel,
    merged_states: set[FeatureStatus],
) -> bool:
    for dep_id in feature.depends_on:
        dep = project.features.get(dep_id)
        if dep is None or dep.status not in merged_states:
            return False
    return True


def resolve_eligible_tasks(board_state: BoardState) -> list[EligibleTask]:
    """Return queued tasks whose dependencies are satisfied, ordered for execution."""
    merged_states = {FeatureStatus.MERGED, FeatureStatus.CLOSED}
    eligible: list[EligibleTask] = []
    for project in board_state.projects.values():
        for feature in project.features.values():
            tasks_by_id = feature.tasks
            if not _feature_deps_merged(feature, project, merged_states):
                continue
            for task in tasks_by_id.values():
                if task.status != TaskStatus.QUEUED:
                    continue
                if not _deps_done(task, tasks_by_id):
                    continue
                eligible.append(EligibleTask(project, feature, task))

    eligible.sort(
        key=lambda e: (
            _dag_depth(e.task, e.feature.tasks),
            e.task.ready_at is None,
            e.task.ready_at or datetime.min,
        )
    )
    return eligible
