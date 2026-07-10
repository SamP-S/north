# 047 — Deferred-items review: v1.0 pulls

Working through the roadmap's deferred sections (2026-07-07/08 review of
`docs/design/99_roadmap.md`) one item at a time. This plan collects what gets
pulled into v1.0; items stay unchecked until the feature lands. Update the
roadmap as each decision is recorded.

## Decisions so far

- **TUI yank (`y`)** — approved. Yanks the **bare task id only** (no
  newline), via OSC 52 (terminal-handled clipboard, no dependencies, no
  shell-outs). No put/paste: yank is a bridge out to the shell, the TUI has
  no paste target, and OSC 52 *read* is blocked by most terminals for
  security. Fires from the focused card (board), selected row (list), and
  the open task popup; status bar confirms `yanked <id>`.
- **`depends_on` pass** — approved (2026-07-08) as a three-level enforcement
  model:
  - New config key **`deps_enforcement: hint | validated | strict`**,
    default **validated**. Enforcement affects writes only — no stored data,
    so switching levels never needs migration. Read per mutation like every
    config value.
  - **Resolved** means the dependency is status `done` or state `archive`
    (terminal state counts as done). One definition shared by CLI and TUI
    (move the TUI `!`-tag logic into `internal/tasks`).
  - **hint**: never refuse; warn on dangling ids (forward refs allowed —
    warning notes the id will bind to whatever gets it), self-refs, cycles,
    and finishing/starting with unmet deps. Delete leaves dangling refs
    (warned).
  - **validated** (default): graph well-formedness is enforced — refuse
    dangling ids, self-refs, and cycles on write (`invalid`); delete heals
    dependents (drops the deleted id, stderr note, same lock hold + same
    auto_commit). Workflow order is advisory: `move done`/`in_progress`
    with unmet deps warns.
  - **strict**: validated, plus `move done`/`in_progress` with unmet deps is
    refused (`conflict`; error lists the unmet deps and the way out).
    `archive` always allowed (terminal = abandon). No healing at load time
    at any level — merged-in damage stays doctor's domain.
  - Cycles at validated are **refused, not warned**: a cycle is never
    intentional and a warned-through one permanently poisons the derived
    reads (unmet forever). Hint is the no-guardrails escape hatch.
  - Read side is level-independent: **`task list --deps met|unmet`** (one
    valued flag — avoids the `--blocked` vs `blocked`-status collision).
    Dangling/cycle deps read as unmet. Docs keep "waiting" as the TUI's
    human word for the `!` tag.
  - Mutation `--json` payloads gain a `"warnings"` array (extends the
    delete pattern) so hint/validated warnings reach agents.
  - Doctor: flags dangling + cycles at all levels; `--fix` gains a
    remove-dangling-refs repair. Healing bumps dependents' `updated_at`
    (document).
  - **TUI `w` link modal (key rebound from `L`, 2026-07-08)**: multi-select picker over **all tasks, all
    states** (revised 2026-07-08: archived allowed as candidates — deps are
    provenance as well as gates, the CLI already allows archived deps at
    validated, and in-modal search absorbs the archive-noise concern).
    Three-tier visual language:
    1. normal — draft/active, unresolved: linking creates a real gate;
    2. **✓ resolved** — done or archived: selectable, born satisfied
       (documents lineage, gates nothing); archived additionally dim so
       "done" and "gone" stay distinguishable;
    3. **greyed — invalid** (`— self`, `— cycle`): visible, not selectable;
       space on one is a no-op with a status-bar reason ("would create a
       cycle via 4 → 7"). (Implementation note: invalidity is static per
       open — a candidate cycles iff it is a transitive *dependent* of the
       edited task, and only that task's edges change — so no live
       recompute is needed.) The UI never assembles an illegal set, at any
       enforcement level.
    Ordering: two groups — draft + active first, archive at the bottom —
    each in the **root-owned sort order** (the `o` picker state; default id
    descending), so the newest tasks — the ones most likely needing deps
    set — sit at the top, and a re-sorted TUI re-sorts the picker too. **In-modal search**: `/` opens a filter input with the same
    grammar as the main views (live narrowing; esc clears/returns to
    list-nav; enter keeps the filter and returns to list-nav) and the same
    matcher (id/title/assignee/labels/body — one search behaviour
    everywhere); filtering hides non-matches including greyed entries;
    while the input is focused, space types rather than toggles (explicit
    focused/unfocused mode split, as the main views do — cover in tests).
    Entries `id title (status:state)`, current deps pre-checked (existing
    deps are always present since all states are listed — apply can never
    silently drop an archived dep); space toggles, enter applies (one Edit
    under the lock), `clear all` entry, esc cancels. `G` is taken (jump to
    bottom); `T` reserved for a possible future read-only dependency tree
    view.
- **undo/redo** — deferred to v2.0 (roadmap updated 2026-07-08); git is the
  undo meanwhile.

## Files to modify (grows as decisions land)

- `internal/board/board.go` — `deps_enforcement` config key + validation.
- `internal/tasks/` — dep-resolution helper (from TUI), enforcement plumbing
  in Create/Edit/SetStatus/Delete, delete healing, warnings surface;
  doctor dangling-ref fix.
- `internal/cli/` — `--deps met|unmet` on task list, config key, JSON
  warnings on mutations.
- `internal/tui/` — `y` yank (OSC 52), `w` link modal, `!` tag reuses the
  shared resolver; keys/help/footer.
- Tests in each package; `README.md`, `docs/design/02/03/05/06`, SKILL.md.

## Todo

1. [x] **TUI yank** — `y` yanks the selected task's bare id via OSC 52;
   status-bar confirmation; board view, list view, and task popup; help row
   + README bullet; behavioural tests.
2. [x] **deps: shared resolver + config key** — move dep resolution into
   `internal/tasks`; add `deps_enforcement` (default validated) to config +
   CLI config cmd.
3. [x] **deps: write-side enforcement** — three levels across
   create/edit/move/delete incl. delete healing, self-ref/cycle refusal,
   JSON warnings channel; tests per level.
4. [x] **deps: read side** — `task list --deps met|unmet`; doctor
   remove-dangling-refs fix; TUI `!` tag on the shared resolver.
5. [x] **TUI `w` link modal** — picker as specified (all states; ✓
   resolved / dim archived / greyed invalid tiers; in-modal `/` search with
   the shared matcher; static grey-out — see implementation note above;
   cycle-safe apply); behavioural tests incl. archived-dep round-trip and
   space-vs-search focus handling.
6. [x] **Docs** — README, design docs 02/03/05/06, SKILL.md (incl. strict
   refusal guidance for agents); roadmap pruning.
7. [x] **Gate** — `make vet && make test` after each landed item.

## Change history

- [2026-07-07] Plan opened during the deferred-items review: yank decided
  (id-only, OSC 52, no put); depends-on TUI modal noted for the depends_on
  review.
- [2026-07-08] depends_on pass decided in full (three-level
  `deps_enforcement`, validated default, cycles enforced at validated,
  `--deps met|unmet`, `L` link modal). undo/redo moved to v2.0 in the
  roadmap. Todos restructured accordingly.
- [2026-07-08] Review paused to prioritise the deps work (remaining list
  items — task pick, custom statuses, templates, multi-select — stay in the
  roadmap's deferred sections). L modal refined: archived tasks excluded
  from candidates, existing deps always shown (incl. archived, dim/✓,
  untogglable), invalid choices greyed with reasons instead of hidden.
- [2026-07-08] L modal revised again: archived tasks *are* candidates
  (provenance links, CLI symmetry) rendered ✓-resolved + dim, archive
  sorted last; in-modal `/` search added (shared matcher, main-view
  grammar) — search is what makes all-states viable. Picker ordering
  follows the root sort state (default id descending).
- [2026-07-08] Implemented in full (todos 1–7): yank via termenv OSC 52
  (bubbletea v1 has no SetClipboard); Snapshot.UnmetDeps +
  TransitiveDependents as the shared resolver; deps_enforcement config key;
  Create/Edit/SetStatus/Delete now return advisory warnings (TUI notices
  escalate to yellow, JSON payloads carry "warnings"); delete healing under
  the held lock; --deps met|unmet; doctor --fix strips dangling refs; L
  modal per spec (grey-out proved static — cycle iff transitive dependent).
  Tests: 10 new tasks-level, 2 CLI, 5 TUI. Docs: README, 02/03/05/06,
  SKILL.md, roadmap pruned. Gate green + live smoke of all three levels.
- [2026-07-08] Keys rebound after review: doctor `D`→`x`, link `L`→`w`
  (shift-pairs whose overlay action is unrelated to the base key — `d`
  delete, `l` right — read as conflicts; `g`/`G` stays, same action). Footer
  hints now list `w link`, `y yank`, `x doctor` (they were help-overlay
  only, which made the features look absent).
- [2026-07-09] TUI themes decided and implemented as plan 048: three
  built-in presets (`default`/`saturated`/`high-contrast`) via a new
  user-level `~/.north/config.yml`, not `north/config.yml` — no `mono`
  preset, no `NORTH_THEME` env override. Roadmap's remaining deferred
  items — named task templates, TUI multi-select — moved to the v2.0-era
  section.
