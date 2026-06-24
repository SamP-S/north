"""Root logging setup for the North MCP server.

uvicorn only wires its own loggers; without this, North app logs reach stderr
through ``logging.lastResort`` at WARNING+ only. Called once from the app
lifespan.
"""

import logging


def configure_logging(level: int = logging.INFO) -> None:
    """Configure root logging with a single stream handler."""
    stream = logging.StreamHandler()
    stream.setFormatter(logging.Formatter("%(asctime)s %(levelname)s %(name)s: %(message)s"))
    root = logging.getLogger()
    root.setLevel(level)
    root.handlers = [stream]
