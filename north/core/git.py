"""Optional local git commit for board mutations.

Only used when ``auto_commit: true`` in ``north/config.yml``. North never
pushes or pulls — remote git is entirely the user's concern. Best-effort: if
the board is not inside a git repo, committing is silently skipped.
"""

import logging
from pathlib import Path

import git

_logger = logging.getLogger(__name__)


def commit_board(
    board: Path,
    message: str,
    paths: list[Path],
    removed: list[Path] | None = None,
) -> None:
    """Stage the given paths (and removals) and commit them locally.

    A no-op when the board is not in a git work tree.
    """
    try:
        repo = git.Repo(board, search_parent_directories=True)
    except git.exc.InvalidGitRepositoryError:
        _logger.warning("auto_commit is on but %s is not in a git repo; skipping commit", board)
        return
    root = Path(repo.working_tree_dir or board).resolve()
    if paths:
        repo.index.add([str(p.resolve().relative_to(root)) for p in paths if p.exists()])
    if removed:
        existing = [p for p in removed if not p.exists()]
        if existing:
            repo.index.remove([str(p.resolve().relative_to(root)) for p in existing], r=True)
    if repo.is_dirty(index=True, working_tree=False) or paths or removed:
        repo.index.commit(message)
