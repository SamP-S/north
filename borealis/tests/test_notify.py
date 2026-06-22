"""Notifier core: dedupe, rate cap, background sending, transport fallback.

No test performs network I/O — transports are fakes or the log transport.
"""

import logging
import threading

from borealis.service import events
from borealis.service.notify import (
    LogTransport,
    Notifier,
    NotifyLogHandler,
    TelegramTransport,
    build_transport,
)


class FakeClock:
    def __init__(self) -> None:
        self.now = 1000.0

    def monotonic(self) -> float:
        return self.now


class FakeTransport:
    def __init__(self) -> None:
        self.sent: list[str] = []
        self.lock = threading.Lock()

    def send(self, text: str) -> None:
        with self.lock:
            self.sent.append(text)


class FailingTransport:
    def __init__(self) -> None:
        self.calls = 0

    def send(self, text: str) -> None:
        self.calls += 1
        raise RuntimeError("boom")


def _notifier(transport, clock, monkeypatch, **kwargs) -> Notifier:
    monkeypatch.setattr("borealis.service.notify.time", clock)
    return Notifier(transport, **kwargs)


def test_sends_through_transport(monkeypatch):
    transport = FakeTransport()
    notifier = _notifier(transport, FakeClock(), monkeypatch)
    notifier.notify("feature_review", key="p/f", text="[feature_review] p/f")
    notifier.drain()
    assert transport.sent == ["[feature_review] p/f"]


def test_dedupe_collapses_within_window(monkeypatch):
    transport = FakeTransport()
    clock = FakeClock()
    notifier = _notifier(transport, clock, monkeypatch, dedupe_window_s=300.0)
    for _ in range(5):
        notifier.notify("task_failed", key="p/f/001", text="t")
    clock.now += 10.0
    notifier.notify("task_failed", key="p/f/001", text="t")
    notifier.drain()
    assert len(transport.sent) == 1


def test_dedupe_expires_after_window(monkeypatch):
    transport = FakeTransport()
    clock = FakeClock()
    notifier = _notifier(transport, clock, monkeypatch, dedupe_window_s=300.0)
    notifier.notify("task_failed", key="p/f/001", text="t")
    clock.now += 301.0
    notifier.notify("task_failed", key="p/f/001", text="t")
    notifier.drain()
    assert len(transport.sent) == 2


def test_different_keys_not_deduped(monkeypatch):
    transport = FakeTransport()
    notifier = _notifier(transport, FakeClock(), monkeypatch)
    notifier.notify("task_failed", key="p/f/001", text="a")
    notifier.notify("task_failed", key="p/f/002", text="b")
    notifier.drain()
    assert transport.sent == ["a", "b"]


def test_rate_cap_drops_excess(monkeypatch, caplog):
    transport = FakeTransport()
    notifier = _notifier(
        transport, FakeClock(), monkeypatch, rate_limit_per_min=3
    )
    for i in range(10):
        notifier.notify("kind", key=str(i), text=f"n{i}")
    notifier.drain()
    assert len(transport.sent) == 3
    assert any("rate cap" in r.message for r in caplog.records)


def test_rate_cap_window_slides(monkeypatch):
    transport = FakeTransport()
    clock = FakeClock()
    notifier = _notifier(transport, clock, monkeypatch, rate_limit_per_min=2)
    notifier.notify("kind", key="1", text="a")
    notifier.notify("kind", key="2", text="b")
    notifier.notify("kind", key="3", text="dropped")
    clock.now += 61.0
    notifier.notify("kind", key="4", text="c")
    notifier.drain()
    assert transport.sent == ["a", "b", "c"]


def test_transport_failure_does_not_kill_worker(monkeypatch):
    transport = FailingTransport()
    notifier = _notifier(transport, FakeClock(), monkeypatch)
    notifier.notify("kind", key="1", text="a")
    notifier.drain()
    assert transport.calls == 1
    # worker survives: a later notification still reaches the transport
    notifier.notify("kind", key="2", text="b")
    notifier.drain()
    assert transport.calls == 2


def test_build_transport_log_default():
    assert isinstance(build_transport("log", "", ""), LogTransport)


def test_build_transport_telegram_configured():
    transport = build_transport("telegram", "123:abc", "42")
    assert isinstance(transport, TelegramTransport)


def test_build_transport_telegram_unconfigured_degrades_to_log(caplog):
    transport = build_transport("telegram", "", "")
    assert isinstance(transport, LogTransport)
    assert any("missing" in r.message for r in caplog.records)


def test_build_transport_unknown_degrades_to_log():
    assert isinstance(build_transport("pigeon", "", ""), LogTransport)


def _record(
    name: str, msg: str, level: int = logging.WARNING, args: tuple = ()
) -> logging.LogRecord:
    return logging.LogRecord(name, level, "f.py", 1, msg, args, None)


def test_log_handler_forwards_warning(monkeypatch):
    transport = FakeTransport()
    notifier = _notifier(transport, FakeClock(), monkeypatch)
    handler = NotifyLogHandler(notifier)
    handler.handle(_record("borealis.service.api", "merge conflict for %s", args=("x",)))
    notifier.drain()
    assert transport.sent == ["[WARNING] borealis.service.api: merge conflict for x"]


def test_log_handler_ignores_info(monkeypatch):
    transport = FakeTransport()
    notifier = _notifier(transport, FakeClock(), monkeypatch)
    handler = NotifyLogHandler(notifier)
    handler.handle(_record("borealis.service.api", "all fine", level=logging.INFO))
    notifier.drain()
    assert transport.sent == []


def test_log_handler_dedupes_by_template(monkeypatch):
    transport = FakeTransport()
    clock = FakeClock()
    notifier = _notifier(transport, clock, monkeypatch)
    handler = NotifyLogHandler(notifier, dedupe_window_s=3600.0)
    # a retry loop: same template, varying args → one notification
    for attempt in range(5):
        handler.handle(_record("borealis.x", "retry %d failed", args=(attempt,)))
    handler.handle(_record("borealis.x", "another problem"))
    notifier.drain()
    assert transport.sent == [
        "[WARNING] borealis.x: retry 0 failed",
        "[WARNING] borealis.x: another problem",
    ]


def test_log_handler_dedupe_expires(monkeypatch):
    transport = FakeTransport()
    clock = FakeClock()
    notifier = _notifier(transport, clock, monkeypatch)
    handler = NotifyLogHandler(notifier, dedupe_window_s=3600.0)
    handler.handle(_record("borealis.x", "boom"))
    clock.now += 3601.0
    handler.handle(_record("borealis.x", "boom"))
    notifier.drain()
    assert len(transport.sent) == 2


def test_log_handler_excludes_notify_machinery(monkeypatch):
    transport = FakeTransport()
    notifier = _notifier(transport, FakeClock(), monkeypatch)
    handler = NotifyLogHandler(notifier)
    handler.handle(_record("borealis.service.notify", "send failed"))
    notifier.drain()
    assert transport.sent == []


def test_emit_event_routes_through_notifier(monkeypatch):
    transport = FakeTransport()
    notifier = Notifier(transport)
    events.set_notifier(notifier)
    try:
        events.emit_event("conversation_shipped", project="p", conversation="001")
        notifier.drain()
        assert transport.sent == ["[conversation_shipped] conversation=001 project=p"]
    finally:
        events.set_notifier(None)


def test_emit_event_summary_becomes_message(monkeypatch):
    transport = FakeTransport()
    notifier = Notifier(transport)
    events.set_notifier(notifier)
    try:
        events.emit_event(
            "feature_review",
            project="p",
            feature="feat-x",
            summary="feat-x ready: 4 tasks, +812/-147, gates green",
        )
        notifier.drain()
        assert transport.sent == [
            "[feature_review] feat-x ready: 4 tasks, +812/-147, gates green"
        ]
    finally:
        events.set_notifier(None)
