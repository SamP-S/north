"""Output rendering for the CLI: human, ``--plain``, and ``--json``.

``--plain`` is stable, tab/line-oriented text for scripts; ``--json`` is the
structured `Task.to_dict()` shape. The default is friendlier human output.
North is agent-driven, so machine-readable output is first-class.
"""

import json
from collections.abc import Sequence

from north.core.models import Task


def render_task_list(tasks: Sequence[Task], *, plain: bool, as_json: bool) -> str:
    if as_json:
        return json.dumps([_summary(t) for t in tasks], indent=2)
    if plain:
        return "\n".join(f"{t.id}\t{t.status}\t{t.title}" for t in tasks)
    if not tasks:
        return "(no tasks)"
    width = max(len(t.id) for t in tasks)
    lines = []
    for t in tasks:
        flag = " (archived)" if t.archived else ""
        lines.append(f"{t.id:<{width}}  {t.status:<12} {t.title}{flag}")
    return "\n".join(lines)


def render_task(task: Task, *, plain: bool, as_json: bool) -> str:
    if as_json:
        return json.dumps(task.to_dict(), indent=2)
    fields = [
        f"id:         {task.id}",
        f"title:      {task.title}",
        f"status:     {task.status}{' (archived)' if task.archived else ''}",
        f"agent:      {task.agent}",
        f"labels:     {', '.join(task.labels)}",
        f"depends_on: {', '.join(task.depends_on)}",
        f"created_at: {task.created_at.isoformat() if task.created_at else ''}",
        f"updated_at: {task.updated_at.isoformat() if task.updated_at else ''}",
    ]
    head = "\n".join(fields)
    body = task.body.strip()
    if plain:
        return f"{head}\n\n{body}" if body else head
    return f"{head}\n\n--- body ---\n{body}" if body else head


def _summary(task: Task) -> dict[str, object]:
    data = task.to_dict()
    data.pop("body", None)
    return data
