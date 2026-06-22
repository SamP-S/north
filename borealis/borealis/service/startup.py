import logging
from pathlib import Path

import git

from borealis.service.board.loader import load_board_state
from borealis.service.board.writer import commit_board, update_task_frontmatter
from borealis.service.models import TaskStatus

_logger = logging.getLogger(__name__)

_PROJECTS_YAML_CONTENT = "schema_version: 1\nprojects: {}\n"


def run_startup_validation(board_path: Path) -> None:
    if not board_path.exists() or not _is_git_repo(board_path):
        from borealis.service.config import settings
        if not settings.board_repo_ssh_url:
            _logger.critical(
                "Board repo not found at %s and BOARD_REPO_SSH_URL is not set. "
                "Add BOARD_REPO_SSH_URL to $NORTH_HOME/.env (~/.north/.env) "
                "then run scripts/install.sh.",
                board_path,
            )
        else:
            _logger.critical(
                "Board repo not found at %s. Run scripts/install.sh to clone it.",
                board_path,
            )
        raise SystemExit(1)

    projects_yaml = board_path / "projects.yaml"
    if not projects_yaml.exists():
        projects_yaml.write_text(_PROJECTS_YAML_CONTENT, encoding="utf-8")
        commit_board(
            board_path,
            "[board:project] init empty projects registry",
            [projects_yaml],
        )

    _reset_in_progress_tasks(board_path)


def _is_git_repo(path: Path) -> bool:
    try:
        git.Repo(path)
        return True
    except Exception:
        return False


def _reset_in_progress_tasks(board_path: Path) -> None:
    state = load_board_state(board_path)
    reset_paths: list[Path] = []
    for project in state.projects.values():
        for feature in project.features.values():
            for task in feature.tasks.values():
                if task.status != TaskStatus.IN_PROGRESS:
                    continue
                update_task_frontmatter(
                    task.task_path, {"status": TaskStatus.QUEUED.value}
                )
                reset_paths.append(task.task_path)
                _logger.info("Reset task %s to queued on startup", task.task_id)

    if reset_paths:
        commit_board(
            board_path,
            "[system:task] reset in_progress tasks on startup",
            reset_paths,
        )
