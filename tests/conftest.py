"""Shared test fixtures for the North CLI.

Provides a MockTransport-backed board client wired into a :class:`NorthContext`
via ``make_ctx``, plus an ``ns`` factory for building ``argparse.Namespace``
inputs. Lifecycle tests don't need a context — they monkeypatch
``lifecycle.subprocess`` and pass ``None`` as the context.
"""

import argparse
from collections.abc import Callable

import httpx
import pytest

from north.cli.clients.board import BoardClient
from north.cli.context import NorthContext

Handler = Callable[[httpx.Request], httpx.Response]
NsFactory = Callable[..., argparse.Namespace]
CtxFactory = Callable[..., NorthContext]


def _board_client(handler: Handler) -> BoardClient:
    """Build a BoardClient whose transport is the given mock handler."""
    client = BoardClient(base_url="http://test")
    client._client = httpx.Client(  # type: ignore[attr-defined]
        base_url="http://test", transport=httpx.MockTransport(handler)
    )
    return client


@pytest.fixture
def ns() -> NsFactory:
    """Return a factory building an ``argparse.Namespace`` from kwargs."""
    return lambda **kwargs: argparse.Namespace(**kwargs)


@pytest.fixture
def make_ctx() -> CtxFactory:
    """Return a factory building a NorthContext with a mock-backed board client.

    Pass ``board=`` a MockTransport handler; the client is only constructed when
    a handler is supplied (mirroring lazy construction).
    """

    def _make(*, board: Handler | None = None) -> NorthContext:
        return NorthContext.for_testing(
            board=_board_client(board) if board is not None else None,
        )

    return _make
