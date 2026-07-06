# 99. Roadmap / Deferred Work

The outcome of the v1.0 roadmap review (2026-07-06/07), which walked every
candidate from `docs/plans/041_v1.0-readyness-analysis.md` one by one. Items
are grouped by decision and sorted by target revision: accepted for v1.0,
deferred (nearest revision first), rejected. Not a wishlist — every entry here
was deliberately decided.

## Known limitations

### Concurrent ID allocation (`board.NextID`)

`board.NextID` scans all task files and returns `max(id) + 1`. It holds no
lock, so two concurrent `north task create` calls can mint the same id and the
second write wins silently.

**Resolution (accepted for v1.0):** an advisory file lock (`north/.lock`)
around mutating commands — see the accepted list below. Until it lands, the
mitigation stands: duplicate ids (race or git merge) are detected on every
load (warnings; the extra task is excluded from the snapshot) and repaired by
`north doctor --fix`, which renumbers the later file.

### `depends_on` is an ordering hint

Existence and referential integrity are enforced on write, delete warns about
dependents, and `north doctor` reports cycles. Writes do not walk the graph —
whether the field should be a formally enforced DAG is an open design
question (see the deferred `depends_on` design pass below).

---

## Accepted for v1.0

Ordered roughly by dependency (foundations first).

1. **Advisory file lock** — `north/.lock` taken briefly around mutating
   commands (`O_CREATE|O_EXCL`, short retry, stale-lock steal). Closes the
   NextID race; stdlib only; no daemon.
2. **Format version stamp** — `init` writes `version: 1` to `config.yml`;
   loading a board with a newer version refuses ("created by a newer north").
   No migration machinery until a `version: 2` exists — deliberately minimal
   so the ambiguity "no key = v1.0 format" never exists. Includes a read-only
   `version` config key: shows in `config list`/`get`, `set` refuses.
3. **Default body template** — `init` scaffolds an editable
   `north/task-template.md` (Summary / Acceptance Criteria / Notes / Changes /
   Comments); bodyless `create` fills from it. Missing or empty file means a
   blank body — the embedded default is only what `init` scaffolds, never a
   runtime fallback. Never parsed back: a scaffold, not schema. The skill
   teaches agents the layout. Supersedes structured body sections, AC
   commands, and comments (all rejected as commands — they are conventions the
   template suggests and the user owns). Future template-content ideas (never
   schema): priority, due date, estimate, references (URLs/SHAs/PRs), modified
   files under Changes. The template stays out of `config` — it is a file,
   not a setting.
4. **CLI batch ids** — comma-delimited (`north task move 2,3,4 done`; arg
   shape unchanged); dedup, continue-on-error with per-id report, non-zero
   exit on any failure.
5. **`--assignee` filter on `task list`** — consistent with
   `--label`/`--status`.
6. **Universal exit-code contract** — mapped from the typed error codes,
   identical in every output mode: 0 success, 1 internal, 2 invalid/usage,
   3 not_found, 4 conflict. Partial batch failure exits with the shared
   failure code, else 1. Plain errors also print the code
   (`error [not_found]: …`). Document in README + skill.
7. **Wider `--plain` task list** — add assignee + labels as tab columns
   (empty when unset) while the contract is still cheap to change; after this
   the rejected `--compact` loses nothing.
8. **`init` next-steps epilogue** — human-mode output ends with a neutral
   "Optional next steps" block: `north skill install`, then
   `north config set auto_commit true` (framed optional, not recommended —
   default stays `false`; per-change commits clutter history). Suppressed
   under `--plain`/`--json`.
9. **`init` scaffolds `north/.gitattributes`** (`* text eol=lf`)
   unconditionally as a board file — protects every future clone from CRLF
   drift (the Windows user who suffers is rarely the one who ran init); no
   platform check, no user-file edits. Doctor warns when missing, `--fix`
   restores. (Already added to this repo's dogfood board.)
10. **Docs hygiene** — reword "freeform" for status/state (it means free
    *movement* within the fixed set — `ParseStatus` already enforces values,
    the docs just read as "any string"); confirm doctor flags hand-edited
    unknown statuses.

## Deferred — flagged for a possible v1.0 look

- **`depends_on` design pass** — decide what the field formally is (ordering
  hint vs enforced DAG), and from that: cycle policy (write-time refusal vs
  doctor-only as today) and the read side — `list --unblocked`/`--blocked`
  filters (reusing the TUI's dep-resolution logic) as the cheap shape of a
  sequences view; a full layered `north sequence` stays with the multi-agent
  story. Needs refinement before any of it is built.

## Deferred — v1.0 release prep

- **vhs-based demo recordings** — README/marketing asset; decide when
  packaging the release.

## Deferred — post-v1.0

- **`north undo`/`redo`** — needs a design review: the git-revert-only shape
  (revert the last trailer-tagged `north:` commit under a stateless
  clean-tree guard; redo = revert the revert) works but is auto_commit-only,
  which felt too conditional; shadow storage was rejected as redundant. Park
  until that tension has a better answer.
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

## Deferred — v2.0-era (distribution)

North is a developer tool; `go install` is the standard, sufficient install
path for v1.0 — distribution machinery isn't necessary yet.

- goreleaser release binaries, Homebrew tap, Scoop, AUR, Nix flake; man pages
  ride along with whatever packaging lands first.
- Skill targets beyond Claude Code/opencode: Codex, Cursor, Gemini CLI dirs
  (kanban-md's registry pattern with auto-detection).
- Docs site (mkdocs/hugo) generated from `docs/design/`.

Already done, pruned from candidates: `north doctor --fix`, self-healing
duplicate-id handling on load, atomic writes (temp+rename), CI test/vet
matrix incl. Windows, `north skill check` (update = rerun `skill install`,
which overwrites), `config get/set/list` with validation.

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
