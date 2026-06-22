import threading
from collections.abc import Callable, Iterator
from contextlib import contextmanager
from pathlib import Path

from fastapi import HTTPException

from north.service.models import BoardState

# Serializes board mutations between API request threads and the supervisor.
board_lock = threading.Lock()

_board_state_getter: Callable[[], BoardState] | None = None
_board_path: Path | None = None


def set_board_context(board_state_getter: Callable[[], BoardState], board_path: Path) -> None:
    global _board_state_getter, _board_path
    _board_state_getter = board_state_getter
    _board_path = board_path


def get_board_context() -> Iterator[tuple[BoardState, Path]]:
    """Yield the board state and path, holding the board lock for the request."""
    if _board_state_getter is None or _board_path is None:
        raise HTTPException(status_code=503, detail="Board not loaded")
    with board_lock:
        yield _board_state_getter(), _board_path


@contextmanager
def acquire_board_context() -> Iterator[tuple[BoardState, Path]]:
    """Board context for non-FastAPI callers (MCP tools), same lock discipline."""
    if _board_state_getter is None or _board_path is None:
        raise HTTPException(status_code=503, detail="Board not loaded")
    with board_lock:
        yield _board_state_getter(), _board_path
