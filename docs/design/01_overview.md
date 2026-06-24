# 1. Overview

North is a task board that lives **inside your project repo**. It is a CLI tool,
not a service: every command reads and writes Markdown files under a `north/`
directory found by walking up from the current directory (like `.git`).

## Principles
- **Files are the source of truth.** A task is a Markdown file; its status is the
  folder it sits in. Anything can read the board without North.
- **Git is yours.** North never pushes or pulls. By default it does not even
  commit (`auto_commit: false`) — your changes show up in `git status`. Opt in to
  per-change local commits with `auto_commit: true`.
- **Agent-first.** North exists to let humans and agents share one board. Output
  has `--plain`/`--json` modes, and `north init` writes an `AGENTS.md` describing
  how to drive the board.
- **Small and un-rigid.** One object (the task), a fixed six-state lifecycle, a
  free-form body the user structures however they like.

## What North is not
- Not a server/daemon (the MCP server is optional and on-demand).
- Not a multi-project/feature/epic tracker — just a flat list of tasks. Group and
  sequence with `depends_on` and notes in the body.
- Not opinionated about the body — acceptance criteria, plans, logs, etc. are
  conventions you choose, not schema North enforces.

## Layout
```
<your-repo>/
  AGENTS.md            # written by `north init`
  north/
    config.yml         # board marker + settings
    draft/ ready/ in_progress/ done/ failed/ blocked/
    archive/
```
