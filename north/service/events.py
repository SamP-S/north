"""Internal event seam for human-gate state changes.

Keep this one function — transports plug in behind it, not beside it.
Events are always logged; the configured notifier transport (log or
telegram) receives a formatted line, deduped per (kind, fields).
A `summary` field, when present, becomes the human-facing message body.
"""

import logging
import threading

from north.service.config import settings
from north.service.notify import Notifier, build_transport

_logger = logging.getLogger(__name__)

_notifier: Notifier | None = None
_notifier_lock = threading.Lock()


def get_notifier() -> Notifier:
    """Return the process-wide notifier, building it from settings on first use."""
    global _notifier
    with _notifier_lock:
        if _notifier is None:
            _notifier = Notifier(
                build_transport(
                    settings.notify_transport,
                    settings.telegram_bot_token,
                    settings.telegram_chat_id,
                ),
                dedupe_window_s=settings.notify_dedupe_window_s,
                rate_limit_per_min=settings.notify_rate_limit_per_min,
            )
        return _notifier


def set_notifier(notifier: Notifier | None) -> None:
    """Replace the process-wide notifier (tests; None resets to lazy default)."""
    global _notifier
    with _notifier_lock:
        _notifier = notifier


def emit_event(kind: str, **fields: str) -> None:
    """Record a human-gate event (e.g. conversation shipped, task blocked on question)."""
    summary = fields.pop("summary", None)
    details = " ".join(f"{key}={value}" for key, value in sorted(fields.items()))
    _logger.info("event %s %s", kind, details)
    text = f"[{kind}] {summary}" if summary else f"[{kind}] {details}"
    get_notifier().notify(kind, key=details, text=text)
