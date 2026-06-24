"""Single source for the agent-facing guidance.

`north init` writes this to ``AGENTS.md`` at the repo root, and
`north instructions` prints it. Keep it short and operational — it tells an
agent how to drive the board via the CLI (and MCP).
"""

_AGENTS = """\
# North — task board for agents

North is a Markdown task board living in the `north/` directory of this repo.
Each task is one file under a status folder (the folder *is* the status):
`draft/ ready/ in_progress/ done/ failed/ blocked/`, plus `archive/`.

## Task file
Frontmatter + free-form body:

```yaml
id: task-12
title: Add login form
status: ready
agent: opus4.8          # optional, opaque executor/provider tag
labels: [auth]          # optional free-form tags
depends_on: [task-4]    # task ids
created_at: ...
updated_at: ...
```

The body is yours to structure (description, plan, notes, blockers, results).

## Lifecycle
`draft -> ready -> in_progress -> done | failed | blocked`;
`failed/blocked/done -> ready` for rework. `draft -> ready` is the human gate.

## CLI
- `north task list [--status S] [--json]` — list tasks
- `north task view <id> [--json]` — read one task (frontmatter + body)
- `north task create "<title>" [--agent A] [--labels ...] [--depends-on ...] [--body ...]`
- `north task edit <id> [--title/--agent/--labels/--depends-on/--body ...]`
- `north task move <id> <status>` — change state (validates the transition)
- `north task archive <id>` / `north cleanup` — move done tasks off the board
- `north board` — counts per status

Use `--json` for stable, parseable output.
"""


def agents_md() -> str:
    """Return the AGENTS.md / `north instructions` text."""
    return _AGENTS
