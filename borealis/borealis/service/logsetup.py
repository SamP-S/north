"""Root logging setup for the Borealis service.

uvicorn's default config only wires its own loggers; without this, Borealis
app logs reach stderr through `logging.lastResort` at WARNING+ only. Called
once from the app lifespan; also attaches the WARNING+ notify forwarder
(service-health notifications, layer a).
"""

import logging

from borealis.service.config import settings
from borealis.service.events import get_notifier
from borealis.service.notify import NotifyLogHandler


def configure_logging(level: int = logging.INFO) -> None:
    """Configure root logging with a stream handler + WARNING+ notify forwarding."""
    stream = logging.StreamHandler()
    stream.setFormatter(
        logging.Formatter("%(asctime)s %(levelname)s %(name)s: %(message)s")
    )
    notify_handler = NotifyLogHandler(
        get_notifier(), dedupe_window_s=settings.log_notify_dedupe_window_s
    )
    root = logging.getLogger()
    root.setLevel(level)
    root.handlers = [stream, notify_handler]
