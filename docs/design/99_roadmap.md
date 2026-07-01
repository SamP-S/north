# 99. Roadmap / Deferred Work

Items explicitly deferred from v1.0. This is not a backlog of features — it is a
record of known design gaps and the reasoning for deferring them.

## Known limitations

### Concurrent ID allocation (`board.NextID`)

`board.NextID` scans all task files and returns `max(id) + 1`. It holds no lock.
Two concurrent `north task create` calls (e.g. two agents running in parallel)
can read the same max, generate the same ID, and the second write wins silently.

**Scope:** single-user local CLI — low risk in practice. The common agent pattern
is sequential task creation, not parallel. Agents issuing parallel `create` calls
should be aware of this limit.

**Deferred because:** fixing requires either a lock file or a different ID scheme
(e.g. UUIDs). UUIDs break the human-readable `task-<n>` contract; a lock file
adds complexity with no benefit for the primary use case.

**Future:** consider a `.north.lock` advisory lock or a monotonic counter file
if concurrent agent use becomes common.

---

### `depends_on` cycle detection

Existence and referential integrity are enforced:
- **On write** (`create`/`edit`): every ID in `depends_on` is validated against
  the board before saving. Unknown IDs are rejected with an `Invalid` error.
- **On delete**: `tasks.Dependents` scans all tasks for references to the deleted
  ID. If any are found, a warning is emitted (CLI: stderr + `"warnings"` in
  `--json`; TUI: folded into the delete confirm modal) and the delete proceeds.

What remains deferred is **cycle detection**: `task-1 → task-2 → task-1` is
currently possible and not caught.

**Deferred because:** cycle detection requires a full graph traversal on every
write. The dependency field is an ordering hint, not an enforced constraint, and
accidental cycles are rare in practice.

**Future:** a `north doctor` / `north lint` command that scans the whole board for
cycles and dangling references would be a cleaner answer than making every write
do a full graph walk.
