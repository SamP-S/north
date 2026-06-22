"""Shared test fixtures for the North CLI.

Provides MockTransport-backed clients wired into a :class:`NorthContext` via
``make_ctx``, plus an ``ns`` factory for building ``argparse.Namespace`` inputs.
Lifecycle tests don't need a context — they monkeypatch ``lifecycle.subprocess``
and pass ``None`` as the context.
"""

import argparse
from collections.abc import Callable

import httpx
import pytest
from north.cli.clients.aurora import AuroraClient
from north.cli.clients.borealis import BorealisClient
from north.cli.context import NorthContext

Handler = Callable[[httpx.Request], httpx.Response]
NsFactory = Callable[..., argparse.Namespace]
CtxFactory = Callable[..., NorthContext]


def _aurora_client(handler: Handler) -> AuroraClient:
    """Build an AuroraClient whose transport is the given mock handler."""
    client = AuroraClient(base_url="http://test")
    client._client = httpx.Client(  # type: ignore[attr-defined]
        base_url="http://test", transport=httpx.MockTransport(handler)
    )
    return client


def _borealis_client(handler: Handler) -> BorealisClient:
    """Build a BorealisClient whose transport is the given mock handler."""
    client = BorealisClient(base_url="http://test")
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
    """Return a factory building a NorthContext with mock-backed clients.

    Pass ``aurora=`` and/or ``borealis=`` MockTransport handlers; only the
    services you supply a handler for get a client (mirroring lazy construction).
    """

    def _make(*, aurora: Handler | None = None, borealis: Handler | None = None) -> NorthContext:
        return NorthContext.for_testing(
            aurora=_aurora_client(aurora) if aurora is not None else None,
            borealis=_borealis_client(borealis) if borealis is not None else None,
        )

    return _make
