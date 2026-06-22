"""North MCP surface: curated tools over the same service layer as REST.

REST stays canonical; MCP is a surface, not the spine (021). One FastMCP
instance per grant set, mounted at `/mcp/{grant}` inside the existing
FastAPI process — each instance registers only its granted tools, so the
tool list per grant is correct by construction. Optional per-grant bearer
tokens are defense-in-depth on a loopback-only service, not an exposure
enabler.

Every tool enters through `acquire_board_context()` (the same lock REST
requests hold) and calls the API-layer functions, so a write through MCP
produces exactly the board commit the REST verb would.
"""

import contextlib
from collections.abc import AsyncIterator, Awaitable, Callable
from functools import wraps
from typing import Any

from fastapi import FastAPI, HTTPException
from mcp.server.fastmcp import FastMCP
from mcp.server.transport_security import TransportSecuritySettings
from starlette.responses import JSONResponse

from north.service.api import comments as comments_api
from north.service.api import conversations as conversations_api
from north.service.api import features as features_api
from north.service.api import tasks as tasks_api
from north.service.api.deps import acquire_board_context
from north.service.models import ConversationStatus, FeatureStatus
from north.service.orchestrator.resolver import resolve_eligible_tasks

_READ_TOOLS = {
    "get_queue",
    "list_features",
    "list_tasks",
    "get_task",
    "get_review",
    "list_conversations",
    "get_conversation",
    "pending_conversations",
    "get_comments",
}

GRANTS: dict[str, set[str]] = {
    "decomposer": _READ_TOOLS | {"create_feature", "create_task"},
    "implementer": _READ_TOOLS | {"add_comment", "split_task"},
    "reviewer": _READ_TOOLS | {"add_comment", "promote_draft", "create_conversation"},
    "cockpit": _READ_TOOLS | {"add_comment", "promote_draft", "create_conversation"},
}


def _tool(fn: Callable[..., Any]) -> Callable[..., Any]:
    """Translate API-layer HTTPExceptions into plain tool errors."""

    @wraps(fn)
    def wrapper(*args: Any, **kwargs: Any) -> Any:
        try:
            return fn(*args, **kwargs)
        except HTTPException as exc:
            raise ValueError(f"{exc.status_code}: {exc.detail}") from exc

    return wrapper


# --- read tools ----------------------------------------------------------------


@_tool
def get_queue(project: str | None = None) -> list[dict]:
    """List in-progress tasks followed by queued tasks in eligible order."""
    with acquire_board_context() as (state, _):
        entries = []
        for proj in state.projects.values():
            if project is not None and proj.name != project:
                continue
            for feat in proj.features.values():
                for task in feat.tasks.values():
                    if str(task.status) == "in_progress":
                        entries.append(
                            {
                                "task_id": task.task_id,
                                "title": task.title,
                                "status": task.status,
                                "project": proj.name,
                                "feature": feat.feature_id,
                            }
                        )
        entries += [
            {
                "task_id": e.task.task_id,
                "title": e.task.title,
                "status": e.task.status,
                "project": e.project.name,
                "feature": e.feature.feature_id,
            }
            for e in resolve_eligible_tasks(state)
            if project is None or e.project.name == project
        ]
        return entries


@_tool
def list_features(project: str) -> list[dict]:
    """List a project's active features."""
    with acquire_board_context() as ctx:
        return features_api.list_features(project, ctx)


@_tool
def list_tasks(project: str, feature: str) -> list[dict]:
    """List the tasks of a feature."""
    with acquire_board_context() as ctx:
        return tasks_api.list_tasks(project, feature, ctx)


@_tool
def get_task(project: str, feature: str, task_id: str) -> dict:
    """Get one task, including its body and result content."""
    with acquire_board_context() as ctx:
        return tasks_api.get_task(project, feature, task_id, ctx)


@_tool
def get_review(project: str | None = None) -> list[dict]:
    """List features awaiting human review."""
    with acquire_board_context() as (state, _):
        return [
            {
                "feature_id": feat.feature_id,
                "title": feat.title,
                "status": feat.status,
                "project": proj.name,
                "branch": feat.branch,
            }
            for proj in state.projects.values()
            if project is None or proj.name == project
            for feat in proj.features.values()
            if feat.status == FeatureStatus.REVIEW
        ]


@_tool
def list_conversations(project: str) -> list[dict]:
    """List a project's conversations."""
    with acquire_board_context() as ctx:
        return conversations_api.list_conversations(project, ctx)


@_tool
def get_conversation(project: str, conversation_id: str) -> dict:
    """Get one conversation, including its body and result content."""
    with acquire_board_context() as ctx:
        return conversations_api.get_conversation(project, conversation_id, ctx)


@_tool
def pending_conversations() -> list[dict]:
    """List pending conversations across projects, oldest first (decomposition queue)."""
    with acquire_board_context() as (state, _):
        pending = [
            {
                "conversation_id": conv.conversation_id,
                "title": conv.title,
                "project": proj.name,
                "created_at": conv.created_at.isoformat() if conv.created_at else None,
            }
            for proj in state.projects.values()
            for conv in proj.conversations.values()
            if conv.status == ConversationStatus.PENDING
        ]
        pending.sort(key=lambda e: (e["created_at"] is None, e["created_at"]))
        return pending


@_tool
def get_comments(project: str, feature: str, task_id: str | None = None) -> list[dict]:
    """Read the comment thread of a task, or of the feature when task_id is omitted."""
    with acquire_board_context() as ctx:
        if task_id is None:
            return comments_api.list_feature_comments(project, feature, ctx)
        return comments_api.list_task_comments(project, feature, task_id, ctx)


# --- write verbs -----------------------------------------------------------------


@_tool
def create_conversation(
    project: str, title: str, content: str = "", source: str = "text"
) -> dict:
    """Ship a condensed conversation onto the board (lands pending)."""
    with acquire_board_context() as ctx:
        body = conversations_api.ConversationCreate(
            title=title, content=content, source=source
        )
        return conversations_api.create_conversation(project, body, ctx)


@_tool
def add_comment(
    project: str,
    feature: str,
    kind: str,
    author: str,
    text: str,
    task_id: str | None = None,
) -> dict:
    """Append a [question]/[answer]/[note] to a task thread, or the feature thread
    when task_id is omitted. Answering a question-blocked task re-readies it."""
    with acquire_board_context() as ctx:
        body = comments_api.CommentCreate(kind=kind, author=author, text=text)
        if task_id is None:
            return comments_api.add_feature_comment(project, feature, body, ctx)
        return comments_api.add_task_comment(project, feature, task_id, body, ctx)


@_tool
def promote_draft(project: str, feature: str, task_id: str | None = None) -> dict:
    """Promote a draft task to ready, or a draft feature to open when task_id is omitted."""
    with acquire_board_context() as ctx:
        if task_id is None:
            return features_api.promote_feature(project, feature, ctx)
        return tasks_api.promote_task(project, feature, task_id, ctx)


@_tool
def create_feature(
    project: str,
    title: str,
    description: str = "",
    depends_on: list[str] | None = None,
    decomposed_from: str | None = None,
) -> dict:
    """Create a feature (lands draft; promote to open before its tasks can run).
    Pass decomposed_from=<conversation id> when decomposing a conversation."""
    with acquire_board_context() as ctx:
        body = features_api.FeatureCreate(
            title=title,
            description=description,
            depends_on=depends_on or [],
            decomposed_from=decomposed_from,
        )
        return features_api.create_feature(project, body, ctx)


@_tool
def create_task(
    project: str,
    feature: str,
    title: str,
    pipeline: str,
    body: str = "",
    depends_on: list[str] | None = None,
    decomposed_from: str | None = None,
) -> dict:
    """Create a task on a feature (lands draft).
    Pass decomposed_from=<conversation id> when decomposing a conversation."""
    with acquire_board_context() as ctx:
        payload = tasks_api.TaskCreate(
            title=title,
            pipeline=pipeline,
            body=body,
            depends_on=depends_on or [],
            decomposed_from=decomposed_from,
        )
        return tasks_api.create_task(project, feature, payload, ctx)


@_tool
def split_task(project: str, feature: str, task_id: str, tasks: list[dict]) -> dict:
    """Replace an oversized task with children; dependencies are relinked atomically.
    Each entry: {"title": str, "body": str = "", "pipeline": str = parent's}."""
    with acquire_board_context() as ctx:
        body = tasks_api.TaskSplit(tasks=[tasks_api.SplitTask(**t) for t in tasks])
        return tasks_api.split_task(project, feature, task_id, body, ctx)


_ALL_TOOLS: dict[str, Callable[..., Any]] = {
    fn.__name__: fn
    for fn in (
        get_queue,
        list_features,
        list_tasks,
        get_task,
        get_review,
        list_conversations,
        get_conversation,
        pending_conversations,
        get_comments,
        create_conversation,
        add_comment,
        promote_draft,
        create_feature,
        create_task,
        split_task,
    )
}


class _TokenGuard:
    """ASGI wrapper requiring `Authorization: Bearer <token>` when a token is set."""

    def __init__(self, app: Callable[..., Awaitable[None]], token: str) -> None:
        self._app = app
        self._token = token

    async def __call__(self, scope: dict, receive: Any, send: Any) -> None:
        if self._token and scope["type"] == "http":
            headers = {k: v for k, v in scope.get("headers", [])}
            expected = f"Bearer {self._token}".encode()
            if headers.get(b"authorization") != expected:
                response = JSONResponse({"detail": "Unauthorized"}, status_code=401)
                await response(scope, receive, send)
                return
        await self._app(scope, receive, send)


def parse_token_map(raw: str) -> dict[str, str]:
    """Parse `grant:token,grant:token` into a mapping (unknown grants rejected)."""
    token_map: dict[str, str] = {}
    for pair in filter(None, (p.strip() for p in raw.split(","))):
        grant, _, token = pair.partition(":")
        if grant not in GRANTS or not token:
            raise ValueError(f"Invalid MCP token entry: {pair!r}")
        token_map[grant] = token
    return token_map


def build_grant_server(
    grant: str, allowed_hosts: list[str] | None = None
) -> FastMCP:
    """Build the FastMCP instance for one grant set."""
    security = TransportSecuritySettings(
        allowed_hosts=allowed_hosts or ["127.0.0.1:*", "localhost:*"]
    )
    server = FastMCP(
        f"north-{grant}",
        instructions=f"North board access with the '{grant}' grant set.",
        stateless_http=True,
        json_response=True,
        transport_security=security,
        streamable_http_path="/",
    )
    for name in sorted(GRANTS[grant]):
        server.add_tool(_ALL_TOOLS[name])
    return server


def mount_mcp(
    app: FastAPI,
    token_map: dict[str, str] | None = None,
    allowed_hosts: list[str] | None = None,
) -> Callable[[], contextlib.AbstractAsyncContextManager[None]]:
    """Mount one MCP server per grant at /mcp/{grant}.

    Returns an async context manager factory the app lifespan must run
    (the SDK's session managers need a running task group).
    """
    token_map = token_map or {}
    servers: list[FastMCP] = []
    for grant in GRANTS:
        server = build_grant_server(grant, allowed_hosts)
        servers.append(server)
        sub_app = server.streamable_http_app()
        app.mount(f"/mcp/{grant}", _TokenGuard(sub_app, token_map.get(grant, "")))

    @contextlib.asynccontextmanager
    async def run_session_managers() -> AsyncIterator[None]:
        async with contextlib.AsyncExitStack() as stack:
            for server in servers:
                await stack.enter_async_context(server.session_manager.run())
            yield

    return run_session_managers


__all__ = ["GRANTS", "build_grant_server", "mount_mcp", "parse_token_map"]
