"""Domain errors for the board.

Surfaces translate these: the CLI renders them as ``error: <message>`` and the
MCP layer turns them into plain tool errors. Keeping them out of FastAPI/HTTP
means the core stays a plain library.
"""


class BoardError(Exception):
    """Base class for all board errors. Carries a short machine code."""

    code = "error"

    def __init__(self, message: str) -> None:
        super().__init__(message)
        self.message = message


class NotFound(BoardError):
    """A task or board could not be found."""

    code = "not_found"


class Conflict(BoardError):
    """The operation is illegal in the current state (e.g. a bad transition)."""

    code = "conflict"


class Invalid(BoardError):
    """The request itself is malformed (e.g. an unknown status)."""

    code = "invalid"
