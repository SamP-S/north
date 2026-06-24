"""`north mcp start|stop|status|run` — manage the on-demand MCP server.

The server is a uvicorn process running ``north.service.main:app``. It is
user-managed (no systemd): ``start`` spawns it detached, ``stop`` signals it,
``status`` checks it, and ``run`` runs it in the foreground. The board is passed
to the server via the ``NORTH_BOARD`` env var; the port comes from
``north/config.yml``.
"""

import argparse
import os
import signal
import subprocess
import sys
from pathlib import Path

from north.cli.errors import CLIError
from north.core.board import load_config, locate_board

_NORTH_HOME = Path.home() / ".north"
_PID_FILE = _NORTH_HOME / "mcp.pid"
_LOG_FILE = _NORTH_HOME / "mcp.log"
_HOST = "127.0.0.1"


def _read_pid() -> int | None:
    if not _PID_FILE.exists():
        return None
    try:
        pid = int(_PID_FILE.read_text().strip())
    except ValueError:
        return None
    return pid if _alive(pid) else None


def _alive(pid: int) -> bool:
    try:
        os.kill(pid, 0)
    except OSError:
        return False
    return True


def start(args: argparse.Namespace) -> int:
    board = locate_board()
    port = load_config(board).mcp_port
    if (pid := _read_pid()) is not None:
        raise CLIError(f"MCP server already running (pid {pid}) — `north mcp stop` first")
    _NORTH_HOME.mkdir(parents=True, exist_ok=True)
    env = {**os.environ, "NORTH_BOARD": str(board)}
    log = _LOG_FILE.open("a", encoding="utf-8")
    proc = subprocess.Popen(
        [sys.executable, "-m", "uvicorn", "north.service.main:app",
         "--host", _HOST, "--port", str(port)],
        stdout=log,
        stderr=log,
        stdin=subprocess.DEVNULL,
        start_new_session=True,
        env=env,
    )
    _PID_FILE.write_text(str(proc.pid))
    print(f"MCP server started (pid {proc.pid}) at http://{_HOST}:{port}/mcp")
    print(f"logs: {_LOG_FILE}")
    return 0


def stop(args: argparse.Namespace) -> int:
    pid = _read_pid()
    if pid is None:
        _PID_FILE.unlink(missing_ok=True)
        print("MCP server is not running.")
        return 0
    os.kill(pid, signal.SIGTERM)
    _PID_FILE.unlink(missing_ok=True)
    print(f"Stopped MCP server (pid {pid}).")
    return 0


def status(args: argparse.Namespace) -> int:
    pid = _read_pid()
    if pid is None:
        print("MCP server: stopped")
        return 0
    port = load_config(locate_board()).mcp_port
    print(f"MCP server: running (pid {pid}) at http://{_HOST}:{port}/mcp")
    return 0


def run(args: argparse.Namespace) -> int:
    import uvicorn

    board = locate_board()
    os.environ["NORTH_BOARD"] = str(board)
    uvicorn.run("north.service.main:app", host=_HOST, port=load_config(board).mcp_port)
    return 0
