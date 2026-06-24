## 99. Planned Features

### 99.1 Overview

The roadmap below lists board-scoped features deferred past the early MVP. Items
have no fixed order. Agent execution, pipelines, and spend tracking are out of
scope for North — they belong to an external agent runtime in a separate
repository.

### 99.2 Roadmap

#### SSE Event Stream

Removed in an earlier API cleanup pass — `GET /api/events`,
`git_watcher.sse_event_queue`, the CLI `logs` command, and the board client's
`sse_stream` were all dead code (no publisher ever populated the queue). Re-add
once a frontend UI (or external client) needs live updates.

- **Board reload** — push `{"type": "board_reloaded"}` from `detect_git_changes`
  when a new commit triggers a full reload, so clients know to re-fetch
  project/feature/task lists
- **Task status changes** — push an event from `update_task_status` (including the
  done → feature-review cascade) so kanban-style views update live
- **Feature status changes** — push an event from `update_feature_status` /
  `requeue_feature` so the review list updates live
- **Queue activity** — push an event when a task becomes `queued` or starts
  running, for a live "current activity" panel
- Keep payloads lightweight (type + project/feature/task ids) — clients re-fetch
  the relevant resource rather than receiving full state over SSE

#### Frontend UI

A web-based board interface served by the North service.

- Project/feature/task timeline tree/graph
- Task kanban boards per feature branch with drag-and-drop status transitions
- Conversation and comment-thread views with git history
- Review list and feature diff viewer

#### Board Testing

- Full demo-project smoke suite as an end-to-end proof of concept
- Exercises: registering a project, shipping a conversation, creating and
  promoting draft features/tasks, the refine rule, split, and board restore from
  the board remote
