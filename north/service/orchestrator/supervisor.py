import asyncio
import logging
from datetime import UTC, datetime
from pathlib import Path

import git

from north.service.api.deps import board_lock
from north.service.config import settings
from north.service.models import BoardState
from north.service.orchestrator.git_watcher import detect_git_changes, sync_remote
from north.service.orchestrator.resolver import promote_ready_tasks

_logger = logging.getLogger(__name__)


class Supervisor:
    def __init__(self, board_path: Path, initial_state: BoardState | None = None) -> None:
        self._board_path = board_path
        self._board_state: BoardState = initial_state if initial_state is not None else BoardState()
        self._last_head: str = ""
        self._board_repo: git.Repo | None = None

    async def run(self) -> None:
        while True:
            try:
                # run in a worker thread: git I/O is sync, and the board lock
                # may be held by an API request thread
                await asyncio.to_thread(self._tick)
            except asyncio.CancelledError:
                _logger.info("Supervisor cancelled; stopping.")
                return
            except Exception:
                _logger.exception("Unexpected error in supervisor loop")
            await asyncio.sleep(settings.poll_interval_s)

    @property
    def board_state(self) -> BoardState:
        return self._board_state

    def _tick(self) -> None:
        with board_lock:
            self._sync_git()
            self._promote()

    def _sync_git(self) -> None:
        if self._board_repo is None:
            self._board_repo = git.Repo(self._board_path)
            self._last_head = self._board_repo.head.commit.hexsha
        sync_remote(self._board_repo)
        self._board_state, self._last_head = detect_git_changes(
            self._board_repo,
            self._board_path,
            self._board_state,
            self._last_head,
        )

    def _promote(self) -> None:
        promote_ready_tasks(self._board_state, self._board_path, datetime.now(UTC))
