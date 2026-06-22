"""HTTP client wrapper for the Borealis service.

Wraps :class:`httpx.Client` bound to the local Borealis service. All transport and
HTTP errors are normalised into :class:`CLIError` so command modules can present
a single human-readable failure path.
"""

import json
import os
from typing import Any

import httpx
from borealis.service.config import settings

from north.cli.clients.errors import CLIError


def _base_url() -> str:
    """Return the base URL for the local Borealis service."""
    port = int(os.environ.get("BOREALIS_PORT", settings.borealis_port))
    return f"http://127.0.0.1:{port}"


class BorealisClient:
    """Thin HTTP client for the Borealis service API.

    Provides ``get``/``post``/``put``/``patch``/``delete`` helpers that raise
    :class:`CLIError` with a human-readable message on connection failure or
    non-2xx responses.
    """

    def __init__(self, base_url: str | None = None, timeout: float = 10.0) -> None:
        self._base_url = base_url or _base_url()
        self._client = httpx.Client(base_url=self._base_url, timeout=timeout)

    def __enter__(self) -> "BorealisClient":
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
                f"could not connect to Borealis service at {self._base_url} "
                "(is it running? try `north service borealis start`)"
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

    def put(self, path: str, body: dict[str, Any] | None = None) -> Any:
        """Issue a PUT request and return the parsed JSON body."""
        return self._request("PUT", path, body=body)

    def patch(self, path: str, body: dict[str, Any] | None = None) -> Any:
        """Issue a PATCH request and return the parsed JSON body."""
        return self._request("PATCH", path, body=body)

    def delete(self, path: str) -> Any:
        """Issue a DELETE request and return the parsed JSON body."""
        return self._request("DELETE", path)


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
