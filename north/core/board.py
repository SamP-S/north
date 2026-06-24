"""Board discovery, scaffolding, and config.

The board is a ``north/`` directory inside the user's project repo, anchored by
``north/config.yml``. It is found by walking up from the current directory, the
same way git finds ``.git``.
"""

import re
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import yaml

from north.core.errors import NotFound
from north.core.instructions import agents_md
from north.core.models import STATUS_DIRS

BOARD_DIRNAME = "north"
CONFIG_NAME = "config.yml"
ARCHIVE_DIR = "archive"
AGENTS_FILE = "AGENTS.md"

_ID_RE = re.compile(r"task-(\d+)")


@dataclass
class Config:
    """Per-board settings from ``north/config.yml``."""

    mcp_port: int = 8001
    auto_commit: bool = False


def locate_board(start: Path | None = None) -> Path:
    """Walk up from ``start`` (default cwd) to find the ``north/`` board dir.

    Returns the path to the ``north/`` directory (the one containing
    ``config.yml``). Raises :class:`NotFound` with a hint if none is found.
    """
    current = (start or Path.cwd()).resolve()
    for directory in (current, *current.parents):
        candidate = directory / BOARD_DIRNAME / CONFIG_NAME
        if candidate.is_file():
            return directory / BOARD_DIRNAME
    raise NotFound("no north board found (run `north init` in your project repo)")


def board_root(board: Path) -> Path:
    """Return the project repo root that contains the board (``north/`` parent)."""
    return board.parent


def init_board(root: Path | None = None) -> Path:
    """Scaffold a board under ``root`` (default cwd): config, folders, AGENTS.md.

    Idempotent — existing files/folders are left untouched. Returns the board dir.
    """
    root = (root or Path.cwd()).resolve()
    board = root / BOARD_DIRNAME
    board.mkdir(exist_ok=True)
    for name in (*STATUS_DIRS, ARCHIVE_DIR):
        (board / name).mkdir(exist_ok=True)
    config_path = board / CONFIG_NAME
    if not config_path.exists():
        write_config(board, Config())
    agents_path = root / AGENTS_FILE
    if not agents_path.exists():
        agents_path.write_text(agents_md(), encoding="utf-8")
    return board


def load_config(board: Path) -> Config:
    """Read ``north/config.yml`` into a :class:`Config` (tolerant of extras)."""
    path = board / CONFIG_NAME
    data: dict[str, Any] = {}
    if path.is_file():
        loaded = yaml.safe_load(path.read_text(encoding="utf-8"))
        if isinstance(loaded, dict):
            data = loaded
    return Config(
        mcp_port=int(data.get("mcp_port", 8001)),
        auto_commit=bool(data.get("auto_commit", False)),
    )


def write_config(board: Path, config: Config) -> Path:
    """Write ``config.yml``. Returns the path."""
    path = board / CONFIG_NAME
    path.write_text(
        yaml.safe_dump(
            {"mcp_port": config.mcp_port, "auto_commit": config.auto_commit},
            default_flow_style=False,
            sort_keys=False,
        ),
        encoding="utf-8",
    )
    return path


def task_files(board: Path, *, include_archive: bool = False) -> list[Path]:
    """All task Markdown files across status folders (and optionally archive)."""
    dirs = [board / s for s in STATUS_DIRS]
    if include_archive:
        dirs.append(board / ARCHIVE_DIR)
    files: list[Path] = []
    for directory in dirs:
        if directory.is_dir():
            files.extend(sorted(directory.glob("task-*.md")))
    return files


def next_id(board: Path) -> str:
    """Return the next free ``task-<n>`` id (max across all folders + 1)."""
    highest = 0
    for path in task_files(board, include_archive=True):
        match = _ID_RE.match(path.name)
        if match:
            highest = max(highest, int(match.group(1)))
    return f"task-{highest + 1}"


def slug(title: str) -> str:
    """Filename-safe slug from a title (Backlog.md-style, dash-separated)."""
    cleaned = re.sub(r"[^A-Za-z0-9]+", "-", title).strip("-")
    return cleaned or "task"


def task_filename(task_id: str, title: str) -> str:
    """The on-disk filename for a task: ``task-12 - Add-login.md``."""
    return f"{task_id} - {slug(title)}.md"
