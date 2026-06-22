"""North CLI entry point.

Defines the top-level ``north`` argument parser and dispatches each subcommand to
its handler. Every handler has the signature ``(args, ctx) -> int`` and receives a
:class:`NorthContext` whose board client is constructed lazily, so a command that
never talks to the service does not depend on it being reachable. All
:class:`CLIError` failures are printed to stderr and exit with status 1.
"""

import argparse
import sys
from collections.abc import Callable

from north.cli.clients.errors import CLIError
from north.cli.commands import (
    comment,
    conversation,
    feature,
    lifecycle,
    observe,
    projects,
    status,
    task,
)
from north.cli.context import NorthContext

Handler = Callable[[argparse.Namespace, NorthContext], int]


def _group_help(parser: argparse.ArgumentParser) -> Handler:
    """Return a handler that prints ``parser``'s help and exits non-zero.

    Used as the ``func`` default for group parsers (``feature``, ``task``, etc.)
    so invoking a group with no subcommand prints its help instead of crashing.
    """

    def _help(_args: argparse.Namespace, _ctx: NorthContext) -> int:
        parser.print_help()
        return 1

    return _help


def _project_opt(parser: argparse.ArgumentParser) -> None:
    """Add the shared ``--project`` filter option."""
    parser.add_argument("--project", help="filter by project name")


def _build_parser() -> argparse.ArgumentParser:
    """Construct the top-level argument parser with all subcommands."""
    parser = argparse.ArgumentParser(prog="north", description="North CLI")
    sub = parser.add_subparsers(dest="command", metavar="<command>")

    _add_status(sub)
    _add_queue(sub)
    _add_service(sub)
    _add_projects(sub)
    _add_feature(sub)
    _add_task(sub)
    _add_conversation(sub)
    _add_comment(sub)

    return parser


def _add_status(sub: "argparse._SubParsersAction[argparse.ArgumentParser]") -> None:
    """Wire ``north status`` (board health)."""
    p_status = sub.add_parser("status", help="show board service status")
    p_status.set_defaults(func=status.status)


def _add_queue(sub: "argparse._SubParsersAction[argparse.ArgumentParser]") -> None:
    """Wire ``north queue``."""
    p_queue = sub.add_parser("queue", help="list active and queued tasks")
    _project_opt(p_queue)
    p_queue.set_defaults(func=observe.queue)


def _add_service_actions(svc_unit: "argparse._SubParsersAction[argparse.ArgumentParser]") -> None:
    """Add the start/stop/restart/enable/disable/status action subparsers.

    The ``unit`` default is set on the parent ``service`` parser and propagates
    here.
    """
    p_start = svc_unit.add_parser("start", help="start the service")
    p_start.set_defaults(func=lifecycle.start)

    p_stop = svc_unit.add_parser("stop", help="stop the service")
    p_stop.set_defaults(func=lifecycle.stop)

    p_restart = svc_unit.add_parser("restart", help="restart the service")
    p_restart.set_defaults(func=lifecycle.restart)

    p_enable = svc_unit.add_parser("enable", help="enable the service at boot")
    p_enable.add_argument("--now", action="store_true", help="also start it now")
    p_enable.set_defaults(func=lifecycle.enable)

    p_disable = svc_unit.add_parser("disable", help="disable the service at boot")
    p_disable.add_argument("--now", action="store_true", help="also stop it now")
    p_disable.set_defaults(func=lifecycle.disable)

    p_status = svc_unit.add_parser("status", help="show OS/process status")
    p_status.set_defaults(func=lifecycle.status)


def _add_service(sub: "argparse._SubParsersAction[argparse.ArgumentParser]") -> None:
    """Wire ``north service <action>`` for the north systemd unit."""
    p_service = sub.add_parser("service", help="manage the north systemd service")
    p_service.set_defaults(func=_group_help(p_service), unit="north")
    _add_service_actions(p_service.add_subparsers(dest="action", metavar="<action>"))


def _add_projects(sub: "argparse._SubParsersAction[argparse.ArgumentParser]") -> None:
    """Wire ``north projects list|show|register|unregister``."""
    p_projects = sub.add_parser("projects", help="manage registered projects")
    p_projects.set_defaults(func=_group_help(p_projects))
    projects_sub = p_projects.add_subparsers(dest="subcommand", metavar="<subcommand>")

    pr_list = projects_sub.add_parser("list", help="list registered projects")
    pr_list.set_defaults(func=projects.list_)

    pr_show = projects_sub.add_parser("show", help="show a project")
    pr_show.add_argument("project")
    pr_show.set_defaults(func=projects.show)

    pr_register = projects_sub.add_parser("register", help="register a new project")
    pr_register.add_argument("ssh_url", help="git SSH URL of the project")
    pr_register.add_argument("--name", help="optional project name override")
    pr_register.add_argument(
        "--base-branch", default="main", help="base branch for merges (default: main)"
    )
    pr_register.add_argument(
        "--auto-merge",
        action="store_true",
        help="automatically merge features when they reach review",
    )
    pr_register.set_defaults(func=projects.register)

    pr_update = projects_sub.add_parser("update", help="update a project's settings")
    pr_update.add_argument("project")
    pr_update.add_argument("--base-branch", help="new base branch for merges")
    am_group = pr_update.add_mutually_exclusive_group()
    am_group.add_argument(
        "--auto-merge", dest="auto_merge", action="store_true", default=None,
        help="enable auto-merge on review",
    )
    am_group.add_argument(
        "--no-auto-merge", dest="auto_merge", action="store_false", default=None,
        help="disable auto-merge on review",
    )
    pr_update.set_defaults(func=projects.update)

    pr_unregister = projects_sub.add_parser("unregister", help="unregister a project")
    pr_unregister.add_argument("project", help="project name")
    pr_unregister.add_argument(
        "-y", "--yes", action="store_true", help="skip confirmation prompt"
    )
    pr_unregister.set_defaults(func=projects.unregister)


def _add_feature(sub: "argparse._SubParsersAction[argparse.ArgumentParser]") -> None:
    """Wire ``north feature`` (board CRUD + list)."""
    p_feature = sub.add_parser("feature", help="manage features")
    p_feature.set_defaults(func=_group_help(p_feature))
    feature_sub = p_feature.add_subparsers(dest="subcommand", metavar="<subcommand>")

    f_create = feature_sub.add_parser("create", help="create a feature")
    f_create.add_argument("project")
    f_create.add_argument("title")
    f_create.add_argument("--description", help="feature description body")
    f_create.add_argument("--depends-on", nargs="*", help="feature dependency ids")
    f_create.set_defaults(func=feature.create)

    f_show = feature_sub.add_parser("show", help="show a feature")
    f_show.add_argument("project")
    f_show.add_argument("feature")
    f_show.set_defaults(func=feature.show)

    f_edit = feature_sub.add_parser("edit", help="edit a feature's fields")
    f_edit.add_argument("project")
    f_edit.add_argument("feature")
    f_edit.add_argument("--title", help="new title")
    f_edit.add_argument("--description", help="new description body")
    f_edit.add_argument("--status", help="new status")
    f_edit.add_argument("--depends-on", nargs="*", help="replace feature dependency ids")
    f_edit.set_defaults(func=feature.edit)

    f_status = feature_sub.add_parser("status", help="set a feature's status")
    f_status.add_argument("project")
    f_status.add_argument("feature")
    f_status.add_argument("status")
    f_status.set_defaults(func=feature.status)

    f_delete = feature_sub.add_parser("delete", help="delete a feature (draft only)")
    f_delete.add_argument("project")
    f_delete.add_argument("feature")
    f_delete.add_argument("-y", "--yes", action="store_true", help="skip confirmation prompt")
    f_delete.set_defaults(func=feature.delete)

    f_requeue = feature_sub.add_parser("requeue", help="re-open a feature")
    f_requeue.add_argument("project")
    f_requeue.add_argument("feature")
    f_requeue.set_defaults(func=feature.requeue)

    f_promote = feature_sub.add_parser("promote", help="promote a draft feature to open")
    f_promote.add_argument("project")
    f_promote.add_argument("feature")
    f_promote.set_defaults(func=feature.promote)

    f_list = feature_sub.add_parser("list", help="list features")
    _project_opt(f_list)
    f_list.add_argument(
        "--archived",
        action="store_true",
        help="include archived features (requires --project)",
    )
    f_list.add_argument(
        "--review", action="store_true", help="only features awaiting review"
    )
    f_list.set_defaults(func=feature.list_)


def _add_task(sub: "argparse._SubParsersAction[argparse.ArgumentParser]") -> None:
    """Wire ``north task create|show|list|status|delete|promote|split``."""
    p_task = sub.add_parser("task", help="manage tasks")
    p_task.set_defaults(func=_group_help(p_task))
    task_sub = p_task.add_subparsers(dest="subcommand", metavar="<subcommand>")

    t_create = task_sub.add_parser("create", help="create a task")
    t_create.add_argument("project")
    t_create.add_argument("feature")
    t_create.add_argument("title")
    t_create.add_argument("--pipeline", required=True, help="pipeline name")
    t_create.add_argument("--body", help="task body text")
    t_create.add_argument("--body-file", help="read task body from a file")
    t_create.add_argument("--depends-on", nargs="*", help="task dependency ids")
    t_create.set_defaults(func=task.create)

    t_show = task_sub.add_parser("show", help="show a task (with result)")
    t_show.add_argument("project")
    t_show.add_argument("feature")
    t_show.add_argument("task_id")
    t_show.set_defaults(func=task.show)

    t_list = task_sub.add_parser("list", help="list a feature's tasks")
    t_list.add_argument("project")
    t_list.add_argument("feature")
    t_list.add_argument("--status", help="filter by task status")
    t_list.set_defaults(func=task.list_)

    t_edit = task_sub.add_parser("edit", help="edit a task's fields")
    t_edit.add_argument("project")
    t_edit.add_argument("feature")
    t_edit.add_argument("task_id")
    t_edit.add_argument("--title", help="new title")
    t_edit.add_argument("--pipeline", help="new pipeline name")
    t_edit.add_argument("--body", help="new task body text")
    t_edit.add_argument("--body-file", help="read new task body from a file")
    t_edit.add_argument("--status", help="new status")
    t_edit.add_argument("--depends-on", nargs="*", help="replace task dependency ids")
    t_edit.set_defaults(func=task.edit)

    t_status = task_sub.add_parser("status", help="set a task's status")
    t_status.add_argument("project")
    t_status.add_argument("feature")
    t_status.add_argument("task_id")
    t_status.add_argument("status")
    t_status.set_defaults(func=task.status)

    t_delete = task_sub.add_parser("delete", help="delete a task")
    t_delete.add_argument("project")
    t_delete.add_argument("feature")
    t_delete.add_argument("task_id")
    t_delete.add_argument("-y", "--yes", action="store_true", help="skip confirmation prompt")
    t_delete.set_defaults(func=task.delete)

    t_promote = task_sub.add_parser("promote", help="promote a draft task to ready")
    t_promote.add_argument("project")
    t_promote.add_argument("feature")
    t_promote.add_argument("task_id")
    t_promote.set_defaults(func=task.promote)

    t_split = task_sub.add_parser("split", help="split a task into replacement children")
    t_split.add_argument("project")
    t_split.add_argument("feature")
    t_split.add_argument("task_id")
    t_split.add_argument(
        "--tasks-json",
        help='replacement tasks as JSON, e.g. \'[{"title": "Part A", "body": "..."}]\'',
    )
    t_split.add_argument("--tasks-file", help="read replacement tasks JSON from a file")
    t_split.set_defaults(func=task.split)


def _add_conversation(sub: "argparse._SubParsersAction[argparse.ArgumentParser]") -> None:
    """Wire ``north conversation create|list|show|status``."""
    p_conversation = sub.add_parser("conversation", help="manage conversations")
    p_conversation.set_defaults(func=_group_help(p_conversation))
    conversation_sub = p_conversation.add_subparsers(dest="subcommand", metavar="<subcommand>")

    c_create = conversation_sub.add_parser(
        "create", help="ship a condensed conversation onto the board"
    )
    c_create.add_argument("project")
    c_create.add_argument("title")
    c_create.add_argument("--content", help="conversation content text")
    c_create.add_argument("--content-file", help="read conversation content from a file")
    c_create.add_argument(
        "--source", choices=["text", "voice"], default="text", help="conversation source"
    )
    c_create.set_defaults(func=conversation.create)

    c_list = conversation_sub.add_parser("list", help="list conversations")
    c_list.add_argument("project", nargs="?", help="project name (omit only with --pending)")
    c_list.add_argument(
        "--pending",
        action="store_true",
        help="show the cross-project pending decomposition queue",
    )
    c_list.set_defaults(func=conversation.list_)

    c_show = conversation_sub.add_parser("show", help="show a conversation")
    c_show.add_argument("project")
    c_show.add_argument("conversation_id")
    c_show.set_defaults(func=conversation.show)

    c_status = conversation_sub.add_parser("status", help="set a conversation's status")
    c_status.add_argument("project")
    c_status.add_argument("conversation_id")
    c_status.add_argument("status")
    c_status.set_defaults(func=conversation.status)


def _add_comment(sub: "argparse._SubParsersAction[argparse.ArgumentParser]") -> None:
    """Wire ``north comment add|list``."""
    p_comment = sub.add_parser("comment", help="task/feature comment threads")
    p_comment.set_defaults(func=_group_help(p_comment))
    comment_sub = p_comment.add_subparsers(dest="subcommand", metavar="<subcommand>")

    def _comment_target(p: argparse.ArgumentParser) -> None:
        p.add_argument("project")
        p.add_argument("feature")
        p.add_argument("--task-id", help="comment on this task (omit for the feature thread)")

    cm_add = comment_sub.add_parser("add", help="append a comment to a thread")
    _comment_target(cm_add)
    cm_add.add_argument("--kind", choices=["question", "answer", "note"], default="note")
    cm_add.add_argument("--author", default="cli")
    cm_add.add_argument("text")
    cm_add.set_defaults(func=comment.add)

    cm_list = comment_sub.add_parser("list", help="print a thread")
    _comment_target(cm_list)
    cm_list.set_defaults(func=comment.list_)


def main(argv: list[str] | None = None) -> int:
    """Parse arguments, dispatch the chosen command, and return an exit code."""
    parser = _build_parser()
    args = parser.parse_args(argv)

    if not getattr(args, "func", None):
        parser.print_help()
        return 1

    try:
        with NorthContext() as ctx:
            return int(args.func(args, ctx))
    except CLIError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        return 130


def _entrypoint() -> None:
    """Console-script wrapper that exits the process with the command's code."""
    sys.exit(main())


if __name__ == "__main__":
    _entrypoint()
