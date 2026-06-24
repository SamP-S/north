"""The optional North MCP server.

A small FastAPI app whose only job is to expose the board over MCP at ``/mcp``.
Run it on demand with ``north mcp start`` (or ``uvicorn north.service.main:app``).
There is no REST API and no background work — the board is plain files on disk.
"""

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from fastapi import FastAPI

from north.service.config import settings
from north.service.logsetup import configure_logging
from north.service.mcp import mount_mcp

_run_session_manager = None


@asynccontextmanager
async def _lifespan(app: FastAPI) -> AsyncIterator[None]:
    configure_logging()
    assert _run_session_manager is not None
    async with _run_session_manager():
        yield


app = FastAPI(title="North", lifespan=_lifespan)
_run_session_manager = mount_mcp(app, token=settings.mcp_token)


@app.get("/health")
def health() -> dict[str, str]:
    return {"status": "ok"}
