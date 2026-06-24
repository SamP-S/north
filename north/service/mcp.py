"""North MCP surface: a single board over streamable HTTP at ``/mcp``.

One FastMCP instance exposing the task tools, mounted inside a small FastAPI
app. Every tool calls straight into ``north.core.tasks`` and translates
:class:`BoardError` into a plain tool error. An optional bearer token
(``MCP_TOKEN``) is defense-in-depth on a loopback-only server.

The board is discovered once on disk: ``NORTH_BOARD`` (set by ``north mcp``)
takes precedence, else the server walks up from its working directory.
"""

import contextlib
import os
from collections.abc import AsyncIterator, Awaitable, Callable
from functools import wraps
from pathlib import Path
from typing import Any

from fastapi import FastAPI
from mcp.server.fastmcp import FastMCP
from mcp.server.transport_security import TransportSecuritySettings
from starlette.responses import JSONResponse

from north.core import tasks as core_tasks
from north.core.board import locate_board
from north.core.errors import BoardError
from north.core.models import Task


def get_board() -> Path:
    """Resolve the board directory for the running server."""
    env = os.environ.get("NORTH_BOARD")
    return Path(env) if env else locate_board()


def _tool(fn: Callable[..., Any]) -> Callable[..., Any]:
    """Translate BoardError into a plain tool error (ValueError)."""

    @wraps(fn)
    def wrapper(*args: Any, **kwargs: Any) -> Any:
        try:
            return fn(*args, **kwargs)
        except BoardError as exc:
            raise ValueError(f"{exc.code}: {exc.message}") from exc

    return wrapper


def _summary(task: Task) -> dict[str, Any]:
    data = task.to_dict()
    data.pop("body", None)
    return data


@_tool
def list_tasks(status: str | None = None, archived: bool = False) -> list[dict[str, Any]]:
    """List tasks (without bodies). Filter by status; include archived ones."""
    tasks = core_tasks.list_tasks(get_board(), status=status, archived=archived)
    return [_summary(t) for t in tasks]


@_tool
def get_task(task_id: str) -> dict[str, Any]:
    """Get one task by id, including its body."""
    return core_tasks.get_task(get_board(), task_id).to_dict()


@_tool
def create_task(
    title: str,
    agent: str = "",
    labels: list[str] | None = None,
    depends_on: list[str] | None = None,
    body: str = "",
) -> dict[str, Any]:
    """Create a task (lands in draft)."""
    return core_tasks.create_task(
        get_board(), title, agent=agent, labels=labels, depends_on=depends_on, body=body
    ).to_dict()


@_tool
def set_task_status(task_id: str, status: str) -> dict[str, Any]:
    """Change a task's status (validates the transition)."""
    return core_tasks.move_task(get_board(), task_id, status).to_dict()


@_tool
def edit_task(
    task_id: str,
    title: str | None = None,
    agent: str | None = None,
    labels: list[str] | None = None,
    depends_on: list[str] | None = None,
    body: str | None = None,
) -> dict[str, Any]:
    """Edit a task's fields and/or body."""
    return core_tasks.edit_task(
        get_board(),
        task_id,
        title=title,
        agent=agent,
        labels=labels,
        depends_on=depends_on,
        body=body,
    ).to_dict()


_TOOLS = (list_tasks, get_task, create_task, set_task_status, edit_task)


class _TokenGuard:
    """ASGI wrapper requiring ``Authorization: Bearer <token>`` when a token is set."""

    def __init__(self, app: Callable[..., Awaitable[None]], token: str) -> None:
        self._app = app
        self._token = token

    async def __call__(self, scope: Any, receive: Any, send: Any) -> None:
        if self._token and scope["type"] == "http":
            headers = dict(scope.get("headers", []))
            if headers.get(b"authorization") != f"Bearer {self._token}".encode():
                await JSONResponse({"detail": "Unauthorized"}, status_code=401)(
                    scope, receive, send
                )
                return
        await self._app(scope, receive, send)


def build_server(allowed_hosts: list[str] | None = None) -> FastMCP:
    """Build the single FastMCP instance with all task tools."""
    server = FastMCP(
        "north",
        instructions="North task board.",
        stateless_http=True,
        json_response=True,
        transport_security=TransportSecuritySettings(
            allowed_hosts=allowed_hosts or ["127.0.0.1:*", "localhost:*"]
        ),
        streamable_http_path="/",
    )
    for tool in _TOOLS:
        server.add_tool(tool)
    return server


def mount_mcp(
    app: FastAPI, token: str = "", allowed_hosts: list[str] | None = None
) -> Callable[[], contextlib.AbstractAsyncContextManager[None]]:
    """Mount the MCP server at ``/mcp``; return its session-manager factory."""
    server = build_server(allowed_hosts)
    app.mount("/mcp", _TokenGuard(server.streamable_http_app(), token))

    @contextlib.asynccontextmanager
    async def run_session_manager() -> AsyncIterator[None]:
        async with server.session_manager.run():
            yield

    return run_session_manager


__all__ = ["build_server", "get_board", "mount_mcp"]
