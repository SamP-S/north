"""HTTP client wrapper for the Aurora service.

Wraps :class:`httpx.Client` bound to the local Aurora service. All transport and
HTTP errors are normalised into :class:`CLIError` so command modules can present
a single human-readable failure path.
"""

import json
import os
from collections.abc import Iterator
from typing import Any

import httpx
from aurora.service.config import settings

from north.cli.clients.errors import CLIError


def _base_url() -> str:
    """Return the base URL for the local Aurora service."""
    port = int(os.environ.get("AURORA_PORT", settings.aurora_port))
    return f"http://127.0.0.1:{port}"


class AuroraClient:
    """Thin HTTP client for the Aurora service API.

    Provides ``get``/``post``/``delete`` helpers that raise :class:`CLIError`
    with a human-readable message on connection failure or non-2xx responses,
    plus :meth:`sse_stream` for consuming the server-sent events endpoint.
    """

    def __init__(self, base_url: str | None = None, timeout: float = 10.0) -> None:
        self._base_url = base_url or _base_url()
        self._client = httpx.Client(base_url=self._base_url, timeout=timeout)

    def __enter__(self) -> "AuroraClient":
        return self

    def __exit__(self, *_exc: object) -> None:
        self.close()

    def close(self) -> None:
        """Close the underlying HTTP connection pool."""
        self._client.close()

    def _request(
        self,
        method: str,
        path: str,
        *,
        params: dict[str, Any] | None = None,
        body: dict[str, Any] | None = None,
    ) -> Any:
        try:
            response = self._client.request(method, path, params=params, json=body)
        except httpx.ConnectError as exc:
            raise CLIError(
                f"could not connect to Aurora service at {self._base_url} "
                "(is it running? try `north service aurora start`)"
            ) from exc
        except httpx.HTTPError as exc:
            raise CLIError(f"request to {path} failed: {exc}") from exc
        return self._handle_response(response)

    @staticmethod
    def _handle_response(response: httpx.Response) -> Any:
        if response.is_success:
            if not response.content:
                return None
            return response.json()
        raise CLIError(_format_error(response))

    def get(self, path: str, params: dict[str, Any] | None = None) -> Any:
        """Issue a GET request and return the parsed JSON body."""
        return self._request("GET", path, params=params)

    def post(self, path: str, body: dict[str, Any] | None = None) -> Any:
        """Issue a POST request and return the parsed JSON body."""
        return self._request("POST", path, body=body)

    def delete(self, path: str) -> Any:
        """Issue a DELETE request and return the parsed JSON body."""
        return self._request("DELETE", path)

    def sse_stream(self, path: str) -> Iterator[dict[str, Any]]:
        """Yield decoded JSON event payloads from an SSE endpoint.

        Each yielded dict is the parsed ``data`` field of one SSE event. Empty
        data lines (heartbeats) are skipped. Raises :class:`CLIError` on a
        connection failure.
        """
        try:
            with self._client.stream("GET", path) as response:
                if not response.is_success:
                    response.read()
                    raise CLIError(_format_error(response))
                for line in response.iter_lines():
                    if not line.startswith("data:"):
                        continue
                    data = line[len("data:"):].strip()
                    if not data:
                        continue
                    try:
                        yield json.loads(data)
                    except json.JSONDecodeError:
                        continue
        except httpx.ConnectError as exc:
            raise CLIError(
                f"could not connect to Aurora service at {self._base_url} "
                "(is it running? try `north service aurora start`)"
            ) from exc
        except httpx.HTTPError as exc:
            raise CLIError(f"streaming from {path} failed: {exc}") from exc


def _format_error(response: httpx.Response) -> str:
    """Build a human-readable error string from a non-2xx response."""
    detail: Any = None
    try:
        payload = response.json()
        if isinstance(payload, dict):
            detail = payload.get("detail", payload)
        else:
            detail = payload
    except (json.JSONDecodeError, ValueError):
        detail = response.text.strip() or None
    if detail is None:
        return f"HTTP {response.status_code} from {response.request.url.path}"
    if not isinstance(detail, str):
        detail = json.dumps(detail)
    return f"HTTP {response.status_code}: {detail}"
