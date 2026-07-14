# 99. Roadmap / Deferred Work

The living outcome of the v1.0 roadmap review (2026-07-06/07, from
`docs/plans/041_v1.0-readyness-analysis.md`). Not a wishlist — every entry was
deliberately decided. Details and rationale live in the numbered plans.

## Known limitations

- **Duplicate ids from git merges** — local processes serialise through
  `north/.lock`, but a merge of two branches that each created the same id
  still collides. Detected on every load (warning; the extra file is excluded
  from the snapshot), repaired by `north doctor --fix` (renumbers the later
  file).
- **`depends_on` enforcement is write-side config** (`deps_enforcement:
  hint | validated | strict`, default validated — event matrix in
  [02_board-data-model.md](02_board-data-model.md)). A layered
  `north sequence` view stays deferred with the multi-agent story.

## Deferred

**v1.0 release prep**
- vhs-based demo recordings — README/marketing asset; decide at packaging.

**v2.0-era** (`go install` is the sufficient install path for v1.0)
- Custom-status boards — configurable status list/colors; blocked on the
  migration story for tasks whose status leaves the set (first real customer
  of the `version:` key).
- `north undo`/`redo` — git-revert-only shape works but is auto_commit-only,
  which felt too conditional; parked (git is the undo meanwhile).
- Named task templates (`create --template bugfix`) — wait for usage data.
- TUI multi-select — let CLI batching prove out first.
- goreleaser binaries, Homebrew/Scoop/AUR/Nix, man pages — ride with
  whatever packaging lands first.
- Skill targets beyond Claude Code/opencode (Codex, Cursor, Gemini CLI).
- Docs site generated from `docs/design/`.

## Shipped (ledger)

- v1.0 accepted set (plan 046, 2026-07-07): board lock, `version: 1` stamp,
  task-template.md, batch ids on move/state/delete, `--assignee` filter,
  exit-code contract, wider `--plain` columns, init epilogue,
  `.gitattributes` scaffold, docs hygiene.
- Deferred-review pulls (plan 047, 2026-07-08): `deps_enforcement` +
  `--deps met|unmet` + delete healing, mutation `--json` warnings, doctor
  dangling-ref fix, TUI `y` yank + `w` dependency picker.
- TUI themes (plan 048, 2026-07-09): three presets via user-level
  `~/.north/config.yml`; per-slot colors and theme downloads rejected.
- Multi-agent claims (plans 049/050, 2026-07-12): `next` + `take` (atomic
  select-and-claim; `assignee` + `in_progress` *is* the claim), `max_wip`,
  `NORTH_AGENT`. Deliberately not built: claim fields, lease expiry,
  `require_claim`, `watch --json`, board-location env override.
- v1.0 polish (plan 051) and CLI consistency pass (plan 054, 2026-07-14):
  `take <id>`, `next --limit`, cleanup lock + `--dry-run` (+ JSON `dry_run`
  key), `last_id` no-reuse mark, doctor exit 0, plain row unification,
  `list --limit`.
- Earlier: `doctor --fix`, tolerant duplicate-id loading, atomic writes,
  CI matrix incl. Windows, `skill check`, `config get/set/list`.

## Rejected

Decided out, permanently — do not re-propose without new evidence.

**Task schema** (frontmatter preservation is the answer to custom fields):
priority field; due dates; estimates; references/`modified_files` tracking.

**Body structure** (the body is the user's; the template suggests, never
enforces): acceptance-criteria commands; structured sections with append
flags; a comments command.

**Task model shape** (one object, flat list): parent/subtasks; milestones/
epics (label conventions suffice); recurring tasks (nothing runs unbidden);
manual column ordering; docs/ADRs as object types.

**History & analytics** (git is the log): `.backup/` shadow copies;
`activity.jsonl`/`north log`; `north stats`; `north context` injection
(goes stale instantly); board export/README injection.

**CLI/UX**: `-C <dir>` global flag (discovery walks up); `--compact` third
output mode; `completion install` (auto-editing shell profiles is invasive);
interactive init wizard (breaks non-TTY/agent use); fuzzy search; per-status
WIP limits (the user owns their agent system).

**TUI**: mouse support (keyboard-only by design); filter popups; per-column
WIP display; hide-empty-columns; snapshot tests.

**Constraint-breakers** (no daemon, no network, no MCP, git is yours):
MCP server; web UI; cross-branch task resolution; GitHub Issues sync;
notifications/webhooks/status-change hooks (repo-committed config must never
execute arbitrary commands).
