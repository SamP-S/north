import logging
from pathlib import Path

import git

from north.service.board.loader import load_board_state
from north.service.models import BoardState

_logger = logging.getLogger(__name__)


def sync_remote(board_repo: git.Repo) -> None:
    """Pull remote board changes onto the local branch, rebasing local commits.

    A no-op when the repo has no `origin` remote (e.g. local-only test boards).
    Failures are logged and the rebase aborted; the next poll retries.
    """
    if not any(remote.name == "origin" for remote in board_repo.remotes):
        return
    try:
        board_repo.git.pull("--rebase")
    except git.exc.GitCommandError as exc:
        try:
            board_repo.git.rebase("--abort")
        except git.exc.GitCommandError:
            pass
        _logger.error("Board remote sync failed: %s", exc)


def detect_git_changes(
    board_repo: git.Repo, board_path: Path, board_state: BoardState, last_head: str
) -> tuple[BoardState, str]:
    """Reload the full board state if the repo HEAD has moved."""
    current_head = board_repo.head.commit.hexsha
    if current_head == last_head:
        return board_state, last_head
    new_state = load_board_state(board_path)
    return new_state, current_head
