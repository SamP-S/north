"""Shared CLI context holding lazily-constructed service clients.

:class:`NorthContext` constructs an :class:`AuroraClient` / :class:`BorealisClient`
only on first access (``ctx.aurora`` / ``ctx.borealis``), so a single-service
command never depends on the other service being reachable. Used as a context
manager, it closes whichever clients were actually constructed.
"""

from types import TracebackType

from north.cli.clients.aurora import AuroraClient
from north.cli.clients.borealis import BorealisClient


class NorthContext:
    """Lazy holder for the Aurora and Borealis CLI clients."""

    def __init__(self) -> None:
        self._aurora: AuroraClient | None = None
        self._borealis: BorealisClient | None = None

    @classmethod
    def for_testing(
        cls,
        *,
        aurora: AuroraClient | None = None,
        borealis: BorealisClient | None = None,
    ) -> "NorthContext":
        """Build a context with pre-set clients for tests."""
        ctx = cls()
        ctx._aurora = aurora
        ctx._borealis = borealis
        return ctx

    @property
    def aurora(self) -> AuroraClient:
        """Return the Aurora client, constructing it on first access."""
        if self._aurora is None:
            self._aurora = AuroraClient()
        return self._aurora

    @property
    def borealis(self) -> BorealisClient:
        """Return the Borealis client, constructing it on first access."""
        if self._borealis is None:
            self._borealis = BorealisClient()
        return self._borealis

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
        """Close whichever clients were actually constructed."""
        if self._aurora is not None:
            self._aurora.close()
        if self._borealis is not None:
            self._borealis.close()
