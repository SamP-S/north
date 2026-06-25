# 1. Overview

North is a task board that lives **inside your project repo**. It is a CLI tool,
not a service: every command reads and writes Markdown files under a `north/`
directory found by walking up from the current directory (like `.git`).

## Principles
- **Files are the source of truth.** A task is a Markdown file; its *state* is the
  folder it sits in, its *status* a frontmatter key. Anything can read the board
  without North.
- **Git is yours.** North never pushes or pulls. By default it does not even
  commit (`auto_commit: false`) — your changes show up in `git status`. Opt in to
  per-change local commits with `auto_commit: true`.
- **Agent-first.** North exists to let humans and agents share one board. Output
  has `--plain`/`--json` modes, and `north skill install` drops a skill file into
  your agent's config describing how to drive the board.
- **Small and un-rigid.** One object (the task), two small axes (state + status),
  a free-form body the user structures however they like.

## What North is not
- Not a server/daemon. There is no MCP server and no network surface.
- Not a multi-project/feature/epic tracker — just a flat list of tasks. Group and
  sequence with `depends_on` and notes in the body.
- Not opinionated about the body — acceptance criteria, plans, logs, etc. are
  conventions you choose, not schema North enforces.

## Layout
```
<your-repo>/
  north/
    config.yml         # board marker + settings
    drafts/            # state: draft
    tasks/             # state: active   (status in frontmatter)
    archive/           # state: archive
```
