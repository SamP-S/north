"""Shared CLI context holding a lazily-constructed board client.

:class:`NorthContext` constructs a :class:`BoardClient` only on first access
(``ctx.board``), so a command that never talks to the service does not depend on
it being reachable. Used as a context manager, it closes the client if one was
actually constructed.
"""

from types import TracebackType

from north.cli.clients.board import BoardClient


class NorthContext:
    """Lazy holder for the North board CLI client."""

    def __init__(self) -> None:
        self._board: BoardClient | None = None

    @classmethod
    def for_testing(
        cls,
        *,
        board: BoardClient | None = None,
    ) -> "NorthContext":
        """Build a context with a pre-set client for tests."""
        ctx = cls()
        ctx._board = board
        return ctx

    @property
    def board(self) -> BoardClient:
        """Return the board client, constructing it on first access."""
        if self._board is None:
            self._board = BoardClient()
        return self._board

    def __enter__(self) -> "NorthContext":
        return self

    def __exit__(
        self,
        _exc_type: type[BaseException] | None,
        _exc: BaseException | None,
        _tb: TracebackType | None,
    ) -> None:
        self.close()

    def close(self) -> None:
        """Close the board client if it was constructed."""
        if self._board is not None:
            self._board.close()
