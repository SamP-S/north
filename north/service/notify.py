"""Notification dispatch: dedupe → rate-cap → queue → background transport.

Sending must never block or fail a board mutation: `Notifier.notify` only
does in-memory bookkeeping and a non-blocking queue put; a daemon thread
drains the queue into the transport. Duplicate (kind, key) events within
the dedupe window collapse to one send, and a global rate cap bounds what
can ever reach the phone — a retry loop becomes one notification, not a
storm. Transports are config (021): `log` is the default and the fallback
whenever Telegram is unconfigured.
"""

import logging
import queue
import threading
import time
from collections import deque
from typing import Protocol

import httpx

_logger = logging.getLogger(__name__)

_QUEUE_SIZE = 100
_TELEGRAM_TIMEOUT_S = 10.0


class Transport(Protocol):
    """Delivers one already-formatted notification line."""

    def send(self, text: str) -> None: ...


class LogTransport:
    """Default transport: notifications are log lines."""

    def send(self, text: str) -> None:
        _logger.info("notify: %s", text)


class TelegramTransport:
    """Outbound-only Telegram `sendMessage` (nothing inbound, plain text)."""

    def __init__(self, bot_token: str, chat_id: str) -> None:
        self._url = f"https://api.telegram.org/bot{bot_token}/sendMessage"
        self._chat_id = chat_id

    def send(self, text: str) -> None:
        response = httpx.post(
            self._url,
            json={"chat_id": self._chat_id, "text": text},
            timeout=_TELEGRAM_TIMEOUT_S,
        )
        response.raise_for_status()


class Notifier:
    """Thread-safe notification funnel in front of a transport."""

    def __init__(
        self,
        transport: Transport,
        dedupe_window_s: float = 300.0,
        rate_limit_per_min: int = 20,
    ) -> None:
        self._transport = transport
        self._dedupe_window_s = dedupe_window_s
        self._rate_limit_per_min = rate_limit_per_min
        self._lock = threading.Lock()
        self._last_sent: dict[tuple[str, str], float] = {}
        self._send_times: deque[float] = deque()
        self._cap_logged_at = float("-inf")
        self._queue: queue.Queue[str] = queue.Queue(maxsize=_QUEUE_SIZE)
        self._thread = threading.Thread(
            target=self._worker, name="notifier", daemon=True
        )
        self._thread.start()

    def notify(self, kind: str, key: str, text: str) -> None:
        """Queue a notification; duplicates and over-cap sends are dropped."""
        now = time.monotonic()
        with self._lock:
            dedupe_key = (kind, key)
            last = self._last_sent.get(dedupe_key)
            if last is not None and now - last < self._dedupe_window_s:
                return
            while self._send_times and now - self._send_times[0] > 60.0:
                self._send_times.popleft()
            if len(self._send_times) >= self._rate_limit_per_min:
                # one cap log per window, however many events hit it
                if now - self._cap_logged_at > 60.0:
                    self._cap_logged_at = now
                    _logger.warning("notify rate cap hit; dropping notifications")
                return
            self._last_sent[dedupe_key] = now
            self._send_times.append(now)
        try:
            self._queue.put_nowait(text)
        except queue.Full:
            _logger.warning("notify queue full; dropped: %s", text)

    def drain(self) -> None:
        """Block until every queued notification has been handed to the transport."""
        self._queue.join()

    def _worker(self) -> None:
        while True:
            text = self._queue.get()
            try:
                self._transport.send(text)
            except Exception:
                _logger.exception("notification send failed: %s", text)
            finally:
                self._queue.task_done()


class NotifyLogHandler(logging.Handler):
    """Forward WARNING+ records through the notifier (service-health layer a).

    Dedupes per (logger name, message template) over a generous window so a
    retry loop logging the same warning collapses to one notification.
    Records from the notify machinery itself are excluded — a transport
    failure must never feed back into another send.
    """

    def __init__(
        self,
        notifier: Notifier,
        dedupe_window_s: float = 3600.0,
        exclude_prefix: str = __name__,
    ) -> None:
        super().__init__(level=logging.WARNING)
        self._notifier = notifier
        self._dedupe_window_s = dedupe_window_s
        self._exclude_prefix = exclude_prefix
        self._last_sent: dict[tuple[str, str], float] = {}

    def emit(self, record: logging.LogRecord) -> None:
        # loggers check handler level before handle(); guard direct calls too
        if record.levelno < self.level or record.name.startswith(self._exclude_prefix):
            return
        template = record.msg if isinstance(record.msg, str) else str(record.msg)
        key = (record.name, template)
        now = time.monotonic()
        # logging.Handler.handle holds this handler's lock around emit,
        # so the dict needs no extra locking
        last = self._last_sent.get(key)
        if last is not None and now - last < self._dedupe_window_s:
            return
        self._last_sent[key] = now
        try:
            self._notifier.notify(
                "log",
                key=f"{record.name}:{template}",
                text=f"[{record.levelname}] {record.name}: {record.getMessage()}",
            )
        except Exception:
            self.handleError(record)


def build_transport(
    transport_name: str, telegram_bot_token: str, telegram_chat_id: str
) -> Transport:
    """Build the configured transport, degrading to log when unconfigured."""
    if transport_name == "telegram":
        if telegram_bot_token and telegram_chat_id:
            return TelegramTransport(telegram_bot_token, telegram_chat_id)
        _logger.warning(
            "notify_transport=telegram but token/chat id missing; using log transport"
        )
    elif transport_name != "log":
        _logger.warning(
            "unknown notify_transport %r; using log transport", transport_name
        )
    return LogTransport()
