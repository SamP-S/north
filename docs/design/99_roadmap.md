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
(e.g. UUIDs). UUIDs break the human-readable numeric-id contract; a lock file
adds complexity with no benefit for the primary use case.

**Mitigation (v1.0):** duplicate ids — whether from a race or a git merge — are
detected on every load (they surface as warnings and the extra task is excluded
from the snapshot) and repaired by `north doctor --fix`, which renumbers the
later file to a fresh id.

**Future:** consider a `.north.lock` advisory lock or a monotonic counter file
if concurrent agent use becomes common.

---

### `depends_on` cycle detection — resolved

Existence and referential integrity are enforced on write (`create`/`edit`
validate every referenced id) and delete warns about dependents. Cycle
detection landed with `north doctor`: it walks the dependency graph and
reports any cycle (e.g. `1 → 2 → 1`). Writes still do not run the graph walk —
the dependency field remains an ordering hint, and doctor is the integrity
tool.

---

## Roadmap candidates

Gathered from a v1.0 readiness review (`docs/plans/041_v1.0-readyness-analysis.md`)
against Backlog.md and kanban-md. Deliberately broad — a list to think from, not a
committed backlog. ⚠ marks items that violate North's current design constraints
(no daemon/network/MCP, single binary, git-is-yours, flat task list). Grouped,
roughly most→least aligned with North's positioning.

### A. Board integrity & concurrency (most "bullet-proof" per effort)
- `north doctor` with `--fix` (duplicates, cycles, dangling deps, CRLF, id/slug drift).
- Advisory file lock (`north/.lock`) around create/mutate — fixes NextID race.
- Self-healing IDs on load (kanban-md): repair duplicates, warn on stderr.
- Atomic writes (temp+rename) + optional `north/.backup/` last-N mutations.
- **`north undo`** — revert the last board mutation (trivially implementable when
  `auto_commit: true`: revert the last `north:` commit; without it, from a
  journal file). Very few tools have this; great demo feature.
- Schema/format version key in `config.yml` + forward-migration hooks
  (kanban-md's `version: N` + migrations discipline).

### B. Task model extensions
- `priority` field (`high|medium|low` or configurable). Deferred out of v1.0.
- Due dates (`due: YYYY-MM-DD`) + overdue surfacing in list/board/TUI.
- Estimates (freeform `4h`, `2d`).
- Acceptance-criteria checklist (`- [ ]` items with `--ac`, `--check-ac N`) —
  Backlog.md's signature agent-quality feature; big but proven value.
- Structured body sections (Plan / Notes / Results) with append flags — a middle
  path that keeps "free-form body" while giving agents append targets.
- Parent/subtasks (`12.1`) or lightweight `parent:` field. ⚠ stretches
  "flat list" principle — could stay a display-only grouping.
- Comments (append-only, authored) — human↔agent dialogue on a task.
- Milestones / epics as label conventions or first-class files. ⚠ flat-list.
- Task templates (`north task create --template bugfix`).
- References field (URLs, commit SHAs, PR numbers) + `modified_files` tracking.
- Custom-status boards: configurable status list/colors in `config.yml`
  (explicitly deferred today; the #1 "make it mine" request to expect).
- Recurring tasks (`every: monday`) — probably out of scope, listed for completeness.
- Ordinal/manual ordering within a column (Backlog.md `ordinal` + web drag).

### C. Multi-agent orchestration (North's likely differentiator)
- **Claims**: `claimed_by`/`claimed_at` + `north task claim/release`, claim
  timeout in config — kanban-md's proven answer to parallel agents.
- **`north task pick --claim NAME`** — atomic find+claim of next eligible task.
- `north agent-name` — generate session identity for claims.
- `require_claim` per status (can't be `in_progress` unclaimed).
- WIP limits per status (warn or enforce on `move`).
- Sequences view: topological layers from `depends_on` (`north sequence`) —
  "what can run in parallel right now" for fleet orchestration.
- Cycle detection at write time (cheap once a snapshot type exists) or in doctor.
- `north watch --json` — long-poll/fsnotify stream of board events for
  orchestrators. (Process runs only while invoked; arguably not a daemon, but ⚠-ish.)

### D. Observability & history
- `activity.jsonl` append log + `north log [--task id]` (kanban-md).
- `north stats` / metrics: throughput, lead/cycle time, aging (kanban-md metrics.go).
- `north context [--write-to CLAUDE.md]` — inject a marker-delimited board
  summary into agent instruction files, idempotently (kanban-md `context`).
- Board export to markdown / README injection (Backlog.md `board export --readme`)
  — rejected for v1.0: piping `--json`/`--plain` output already covers this.

### E. CLI & UX growth
- Batch IDs, `--search`, label/agent filters, `-C`, stdin bodies, JSON errors,
  exit-code contract.
- `--compact` one-line output mode (kanban-md measured ~70% token savings vs JSON).
- `north config get/set/list` with validation.
- Man pages (`cobra doc gen`), `north completion install`.
- Interactive `north init` wizard (statuses, auto_commit, skill install, gitattributes).
- Fuzzy search (`north search`) across tasks (+docs/decisions if added).
- Docs & decisions (ADRs) as additional object types (Backlog.md `doc`/`decision`). ⚠ "one object" principle.

### F. TUI growth
The TUI is **keyboard-only by design** — mouse support is a non-goal, not a
candidate.
- Filter popups (status/label/priority), yank task id to clipboard (`y`),
  multi-select, per-column WIP display, themes/config (`tui:` block in
  config.yml), hide-empty-columns, vhs-based demo recordings, snapshot tests.

### G. Ecosystem & distribution
- goreleaser + GitHub releases, Homebrew tap, Scoop, AUR, Nix flake, `go install` docs.
- CI (test/vet matrix incl. Windows — would have caught the CRLF bug).
- Skill targets beyond Claude Code/opencode: Codex, Cursor, Gemini CLI dirs
  (kanban-md's registry pattern with auto-detection).
- `north skill check/update` — compare the stamped skill version against the binary.
- Docs site (mkdocs/hugo) generated from `docs/design/`.

### H. Constraint-breaking (flag explicitly, decide deliberately)
- ⚠ **MCP server** (`north mcp start`, stdio): violates "no MCP"; Backlog.md
  ships one, but the skill+CLI approach is genuinely competitive and cheaper —
  recommend keeping the constraint for v1.0, revisit if agent platforms
  deprecate shell access.
- ⚠ **Web UI / `north browser`**: violates no-daemon; huge surface; skip.
- ⚠ **Cross-branch task resolution** (Backlog.md's mtime-based multi-branch
  view) and remote-branch ID checking: violates "git is yours" and adds the
  most complexity per feature on this list. A cheaper 80% answer for a
  worktree-heavy workflow: `north doctor` detects post-merge duplicates, plus a
  documented convention (create tasks on main, work them on branches).
- ⚠ GitHub Issues import/export sync (network).
- ⚠ Notifications/webhooks/`on_status_change` hook — the hook variant (run a
  local shell command on status change, Backlog.md `onStatusChange`) is actually
  constraint-compatible and cheap; network notifications are not.
