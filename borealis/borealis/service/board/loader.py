import logging
from pathlib import Path

from borealis.service.board.parser import (
    ParseError,
    parse_conversation,
    parse_feature,
    parse_projects_yaml,
    parse_task,
)
from borealis.service.models import BoardState, FeatureModel, ProjectModel

_logger = logging.getLogger(__name__)


def load_board_state(board_path: Path) -> BoardState:
    """Walk the board repo and build the full in-memory BoardState."""
    state = BoardState()

    projects_yaml = board_path / "projects.yaml"
    if projects_yaml.exists():
        try:
            state.projects = parse_projects_yaml(projects_yaml)
        except ParseError as exc:
            _logger.warning("Skipping %s: %s", exc.path, exc.original)

    projects_dir = board_path / "projects"
    if not projects_dir.is_dir():
        return state

    for project_dir in sorted(p for p in projects_dir.iterdir() if p.is_dir()):
        project_name = project_dir.name
        project = state.projects.get(project_name)
        if project is None:
            _logger.warning(
                "Project %r not registered in projects.yaml, skipping", project_name
            )
            continue
        _load_features(project_dir, project)
        _load_conversations(project_dir, project)

    return state


def load_archived_features(board_path: Path, project: str) -> list[FeatureModel]:
    """Parse archived feature directories for a project (read-only, on demand)."""
    archived_dir = board_path / "projects" / project / "board" / "features" / "archived"
    if not archived_dir.is_dir():
        return []
    features: list[FeatureModel] = []
    for feature_dir in sorted(p for p in archived_dir.iterdir() if p.is_dir()):
        feature_file = feature_dir / "_feature.md"
        if not feature_file.is_file():
            continue
        try:
            feature = parse_feature(feature_file)
        except ParseError as exc:
            _logger.warning("Skipping %s: %s", exc.path, exc.original)
            continue
        _load_tasks(feature_dir, feature)
        features.append(feature)
    return features


def _load_features(project_dir: Path, project: ProjectModel) -> None:
    """Parse feature directories for a project and attach them to the project model."""
    active_dir = project_dir / "board" / "features" / "active"
    if not active_dir.is_dir():
        return
    for feature_dir in sorted(p for p in active_dir.iterdir() if p.is_dir()):
        feature_file = feature_dir / "_feature.md"
        if not feature_file.is_file():
            continue
        try:
            feature = parse_feature(feature_file)
        except ParseError as exc:
            _logger.warning("Skipping %s: %s", exc.path, exc.original)
            continue
        project.features[feature.feature_id] = feature
        _load_tasks(feature_dir, feature)


def _load_conversations(project_dir: Path, project: ProjectModel) -> None:
    """Parse conversation files for a project and attach them to the project model."""
    conversations_dir = project_dir / "conversations"
    if not conversations_dir.is_dir():
        return
    for path in sorted(conversations_dir.glob("*.md")):
        if path.name.endswith(".result.md") or path.name.endswith(".thread.md"):
            continue
        try:
            conversation = parse_conversation(path)
        except ParseError as exc:
            _logger.warning("Skipping %s: %s", exc.path, exc.original)
            continue
        project.conversations[conversation.conversation_id] = conversation


def _load_tasks(feature_dir: Path, feature: FeatureModel) -> None:
    """Parse task files for a feature and attach them to the feature model."""
    tasks_dir = feature_dir / "tasks"
    if not tasks_dir.is_dir():
        return
    for path in sorted(tasks_dir.glob("*.md")):
        if path.name.endswith(".result.md") or path.name.endswith(".thread.md"):
            continue
        try:
            task = parse_task(path)
        except ParseError as exc:
            _logger.warning("Skipping %s: %s", exc.path, exc.original)
            continue
        feature.tasks[task.task_id] = task
