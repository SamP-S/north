"""North CLI entry point.

Defines the ``north`` argument parser and dispatches each subcommand to its
handler ``(args) -> int``. Commands operate directly on the in-repo ``north/``
board (discovered by walking up from cwd) — there is no service to reach.
:class:`CLIError` and :class:`BoardError` failures print to stderr and exit 1.
"""

import argparse
import sys
from collections.abc import Callable

from north.cli.commands import board as board_cmd
from north.cli.commands import cleanup as cleanup_cmd
from north.cli.commands import init as init_cmd
from north.cli.commands import instructions as instructions_cmd
from north.cli.commands import mcp as mcp_cmd
from north.cli.commands import task as task_cmd
from north.cli.errors import CLIError
from north.core.errors import BoardError

Handler = Callable[[argparse.Namespace], int]
SubParsers = argparse._SubParsersAction


def _group_help(parser: argparse.ArgumentParser) -> Handler:
    """A handler that prints a group's help and exits non-zero."""

    def _help(_args: argparse.Namespace) -> int:
        parser.print_help()
        return 1

    return _help


def _output_opts(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--plain", action="store_true", help="stable unformatted output")
    parser.add_argument("--json", action="store_true", help="JSON output")


def _build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="north", description="North task board CLI")
    sub = parser.add_subparsers(dest="command", metavar="<command>")

    p_init = sub.add_parser("init", help="scaffold a north/ board in this repo")
    p_init.set_defaults(func=init_cmd.init)

    _add_task(sub)

    p_board = sub.add_parser("board", help="board summary (counts per status)")
    p_board.set_defaults(func=board_cmd.board)

    p_cleanup = sub.add_parser("cleanup", help="archive done tasks")
    p_cleanup.add_argument(
        "--older-than", type=int, default=None, metavar="DAYS",
        help="only archive done tasks older than DAYS",
    )
    p_cleanup.set_defaults(func=cleanup_cmd.cleanup)

    p_instr = sub.add_parser("instructions", help="print agent guidance (AGENTS.md)")
    p_instr.set_defaults(func=instructions_cmd.instructions)

    _add_mcp(sub)
    return parser


def _add_task(sub: "SubParsers[argparse.ArgumentParser]") -> None:
    p_task = sub.add_parser("task", help="manage tasks")
    p_task.set_defaults(func=_group_help(p_task))
    ts = p_task.add_subparsers(dest="subcommand", metavar="<subcommand>")

    t_create = ts.add_parser("create", help="create a task (lands in draft)")
    t_create.add_argument("title")
    t_create.add_argument("--agent", help="executor/provider tag (opaque)")
    t_create.add_argument("--labels", nargs="*", help="free-form labels")
    t_create.add_argument("--depends-on", nargs="*", help="task ids this depends on")
    t_create.add_argument("--body", help="task body text")
    t_create.add_argument("--body-file", help="read task body from a file")
    t_create.set_defaults(func=task_cmd.create)

    t_view = ts.add_parser("view", help="show a task (frontmatter + body)")
    t_view.add_argument("task_id")
    _output_opts(t_view)
    t_view.set_defaults(func=task_cmd.view)

    t_list = ts.add_parser("list", help="list tasks")
    t_list.add_argument("--status", help="filter by status")
    t_list.add_argument("--archived", action="store_true", help="include archived tasks")
    _output_opts(t_list)
    t_list.set_defaults(func=task_cmd.list_)

    t_edit = ts.add_parser("edit", help="edit a task's fields/body")
    t_edit.add_argument("task_id")
    t_edit.add_argument("--title")
    t_edit.add_argument("--agent")
    t_edit.add_argument("--labels", nargs="*", help="replace labels (empty to clear)")
    t_edit.add_argument("--depends-on", nargs="*", help="replace dependencies (empty to clear)")
    t_edit.add_argument("--body")
    t_edit.add_argument("--body-file")
    t_edit.set_defaults(func=task_cmd.edit)

    t_move = ts.add_parser("move", help="change a task's status")
    t_move.add_argument("task_id")
    t_move.add_argument("status")
    t_move.set_defaults(func=task_cmd.move)

    t_archive = ts.add_parser("archive", help="move a task to archive/")
    t_archive.add_argument("task_id")
    t_archive.set_defaults(func=task_cmd.archive)

    t_delete = ts.add_parser("delete", help="delete a task")
    t_delete.add_argument("task_id")
    t_delete.add_argument("-y", "--yes", action="store_true", help="skip confirmation")
    t_delete.set_defaults(func=task_cmd.delete)


def _add_mcp(sub: "SubParsers[argparse.ArgumentParser]") -> None:
    p_mcp = sub.add_parser("mcp", help="manage the on-demand MCP server")
    p_mcp.set_defaults(func=_group_help(p_mcp))
    ms = p_mcp.add_subparsers(dest="action", metavar="<action>")
    ms.add_parser("start", help="start the MCP server (detached)").set_defaults(func=mcp_cmd.start)
    ms.add_parser("stop", help="stop the MCP server").set_defaults(func=mcp_cmd.stop)
    ms.add_parser("status", help="show MCP server status").set_defaults(func=mcp_cmd.status)
    ms.add_parser("run", help="run the MCP server in the foreground").set_defaults(func=mcp_cmd.run)


def main(argv: list[str] | None = None) -> int:
    """Parse arguments, dispatch the chosen command, return an exit code."""
    parser = _build_parser()
    args = parser.parse_args(argv)
    func: Handler | None = getattr(args, "func", None)
    if func is None:
        parser.print_help()
        return 1
    try:
        return int(func(args))
    except (CLIError, BoardError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        return 130


def _entrypoint() -> None:
    """Console-script wrapper that exits with the command's code."""
    sys.exit(main())


if __name__ == "__main__":
    _entrypoint()
