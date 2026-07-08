# 99. Roadmap / Deferred Work

The outcome of the v1.0 roadmap review (2026-07-06/07), which walked every
candidate from `docs/plans/041_v1.0-readyness-analysis.md` one by one. Items
are grouped by decision and sorted by target revision: deferred (nearest
revision first), then rejected. Not a wishlist — every entry here was
deliberately decided. The ten items accepted for v1.0 were implemented on
2026-07-07 (`docs/plans/046_v1.0-accepted.md`) and moved to the done list
below.

## Known limitations

### Duplicate ids from git merges

Concurrent processes on one machine serialise through the advisory
`north/.lock` (landed for v1.0), so `board.NextID` can no longer race
locally. Duplicate ids can still arrive from outside — a git merge of two
branches that each created the same id. They are detected on every load
(warnings; the extra task is excluded from the snapshot) and repaired by
`north doctor --fix`, which renumbers the later file.

### `depends_on` enforcement is per-board config

Settled 2026-07-08 (plan 047): `deps_enforcement: hint | validated | strict`
(default validated) grades write-side enforcement — see
`docs/design/02_board-data-model.md` for the event matrix. The read side is
`task list --deps met|unmet` plus the TUI's `!` tag and `w` link picker, all
sharing one resolution rule (done or archived = resolved). A full layered
`north sequence` view stays deferred with the multi-agent story.

---

## Deferred — v1.0 release prep

- **vhs-based demo recordings** — README/marketing asset; decide when
  packaging the release.

## Deferred — post-v1.0

- **Multi-agent claims cluster** — claims + `pick --claim` + `agent-name` +
  `require_claim` + `watch --json` stand or fall together. kanban-md's
  implementation is not a model to copy (frontmatter claims aren't atomic
  without a lock protocol; timeout expiry can put two agents on one task; the
  claim/assignee split — possession vs intent — needs first-principles
  design). Requires a plan doc covering the concurrency model (lock scope,
  crash recovery, cross-platform atomicity) before any of it is built.
  Meanwhile: the board is plain files — orchestrators can watch `north/` with
  any watcher.
- **Custom-status boards** — configurable status list/colors in `config.yml`;
  the #1 "make it mine" request to expect. Blocked on the migration story: a
  user-defined set must answer what happens to tasks (incl. archived) whose
  status leaves the set — first real customer of the `version:` key.
- **Named task templates** (`create --template bugfix`) — extends the
  accepted `north/task-template.md`; wait for real usage data on how agents
  behave with different bodies before designing.
- **TUI:** yank task id to clipboard (`y` — clipboard portability is real
  surface), multi-select (TUI twin of CLI batching — let batching prove out
  first), themes/config (`tui:` block; interacts with custom statuses).
  The TUI remains keyboard-only by design — mouse support is a non-goal.

## Deferred — v2.0-era

North is a developer tool; `go install` is the standard, sufficient install
path for v1.0 — distribution machinery isn't necessary yet.

- **`north undo`/`redo`** (moved from post-v1.0, 2026-07-08) — needs a design
  review: the git-revert-only shape (revert the last trailer-tagged `north:`
  commit under a stateless clean-tree guard; redo = revert the revert) works
  but is auto_commit-only, which felt too conditional; shadow storage was
  rejected as redundant. Parked until that tension has a better answer;
  meanwhile git is the undo (uncommitted → `git restore`, auto_commit →
  `git revert`).

- goreleaser release binaries, Homebrew tap, Scoop, AUR, Nix flake; man pages
  ride along with whatever packaging lands first.
- Skill targets beyond Claude Code/opencode: Codex, Cursor, Gemini CLI dirs
  (kanban-md's registry pattern with auto-detection).
- Docs site (mkdocs/hugo) generated from `docs/design/`.

Already done, pruned from candidates: `north doctor --fix`, self-healing
duplicate-id handling on load, atomic writes (temp+rename), CI test/vet
matrix incl. Windows, `north skill check` (update = rerun `skill install`,
which overwrites), `config get/set/list` with validation.

Deferred-review pulls implemented for v1.0 (2026-07-08, plan 047): TUI `y`
yank (bare id via OSC 52), the `depends_on` pass — `deps_enforcement`
config (hint/validated/strict, default validated; cycles/self-refs/dangling
refused at validated+, strict refuses done/in_progress with unmet deps,
delete heals dependents at validated+), `task list --deps met|unmet`,
mutation `--json` warnings arrays, doctor `--fix` removes dangling refs,
and the TUI `w` dependency picker (all states, resolved ✓, invalid greyed
with reasons, in-modal filter).

Accepted for v1.0 and implemented (2026-07-07, plan 046): advisory file lock
(`north/.lock`), `version: 1` format stamp (read-only config key, newer boards
refused), default body template (`north/task-template.md`), CLI batch ids on
`move`/`state`/`delete`, `--assignee` filter on `task list`, universal
exit-code contract (0/1/2/3/4 with `error [<code>]:` output), wider `--plain`
task list (assignee + labels columns), `init` next-steps epilogue, `init`
scaffolds `north/.gitattributes` (doctor warns/fixes), docs hygiene
("freeform" reworded; doctor flags unknown statuses as unparseable).

---

## Rejected

Decided out, permanently — do not re-propose without new evidence.

**Task schema** (the frontmatter-preservation guarantee is North's answer to
custom fields — users own their extra keys, North preserves them):
- `priority` field
- Due dates / overdue surfacing
- Estimates (additionally: no command would ever consume the value)
- References field + `modified_files` tracking (template's Changes section is
  the convention; kept as template-content ideas under the accepted template
  entry)

**Body structure** (the body is the user's; the accepted template scaffold
suggests conventions without enforcing them):
- Acceptance-criteria commands (`--ac` / `--check-ac`)
- Structured body sections with append flags
- Comments command (`--append-body` + the template's Comments section cover it)

**Task model shape** (one object, flat list):
- Parent/subtasks — dotted ids or `parent:` field (labels + `depends_on`
  group and sequence)
- Milestones / epics (label conventions, no code needed)
- Recurring tasks (needs a scheduler; nothing in North runs unbidden)
- Ordinal/manual column ordering (`--sort` + `depends_on` cover ordering;
  ordinals are a drag-UI concept North doesn't have)
- Docs & decisions (ADRs) as object types (staying lightweight — tasks only;
  docs are ordinary repo files with no workflow axis)

**History & analytics** (git is the log; auto_commit is the revisioning
mechanism):
- `north/.backup/` shadow copies
- `activity.jsonl` + `north log` (audit with `git log -- north/`)
- `north stats` / flow metrics (enterprise-team analytics; too heavy for a
  lightweight CLI/TUI tool — the data is open in files + git for scripting)
- `north context` — board summary injected into CLAUDE.md etc. (the skill
  queries live state; injected summaries go stale instantly)
- Board export / README injection (the board already lives in the repo;
  adds nothing over `--plain`/`--json`)

**CLI/UX**:
- `-C <dir>` global flag (board discovery walks up; run from the project tree)
- `--compact` output mode (`--plain` is the one-line token-lean format; a
  third format fragments the contract)
- `north completion install` (generation exists; auto-editing shell profiles
  is invasive, standard sourcing recipes suffice)
- Interactive `north init` wizard (prompts break non-TTY/agent use — replaced
  by the accepted next-steps epilogue)
- Fuzzy search `north search` (`--search` + TUI `/` filter cover board scale;
  fuzzy ranking adds a scorer dependency for marginal gain)
- WIP limits per status (the user owns their agent system; North doesn't
  throttle)

**TUI**:
- Filter popups (the `/` live filter covers it)
- Per-column WIP display (WIP limits rejected)
- Hide-empty-columns (overkill)
- Snapshot tests (golden-file maintenance outweighs value; 21 behavioral
  tests + manual smoke tests are sufficient)

**Constraint-breakers** (no daemon, no network, no MCP, git is yours):
- MCP server (skills are sufficient; agent platforms deprecating shell access
  is not a realistic risk)
- Web UI / `north browser` (no daemon; the TUI is the human interface)
- Cross-branch task resolution (violates "git is yours"; doctor's post-merge
  duplicate repair is the answer)
- GitHub Issues import/export sync (no network; the board lives in the repo)
- Notifications/webhooks/`on_status_change` hook (network is out by
  constraint; the local-hook variant means repo-committed config executing
  arbitrary commands — running north never executes anything but north and
  your git)
