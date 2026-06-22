## 10. Testing strategy

Tests live in `tests/` and run with `uv run pytest`. Lint with `uv run ruff
check .`; type-check with `uv run mypy north`.

### 10.1 Unit

- Frontmatter / task / thread parsers
- Dependency resolver + queue ordering (cooldown, shallowest-first)
- Feature-dependency eligibility
- Commit-prefix handling
- Draft gating — created features/tasks land `draft`; promote is the only exit
- Status transition table — illegal jumps rejected with `409`
- Feature lifecycle transitions — `draft → open → in_progress → review → merged/closed`
- Comment threads — typed append, question→answer status flip
- Split — children inherit `depends_on`, dependents re-pointed, parent `superseded`
- Refine rule — a new task on a `review` feature reverts it to `in_progress`
- Gate events — `feature_review` emitted when the `north/brief` note lands
- MCP surface — grant filtering, token auth, read/write parity with REST
- Notifications — dedupe and rate-limit behaviour

### 10.2 Integration

- Board `HEAD` polling — a new commit is detected and the loop reloads
- Board mutations land as one commit each through the API
- Feature → `review` transition when all tasks reach `done`
- Archive-on-merge in the board repo
- Project register / unregister round-trip against the registry

### 10.3 Smoke

Manual, run against a demo project after significant changes (see
[planned features](99_planned-features.md) for the full demo-project plan):
register a project, ship a conversation, create and promote draft features/tasks,
exercise the refine rule and split, and restore the board from its remote.
