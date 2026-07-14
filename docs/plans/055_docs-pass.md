# 055 — Docs pass: accuracy + signal-to-noise

## Summary

Verify every doc against the current code, fix drift, and condense — the code
and CLI surface are self-describing, so docs keep only what they add: design
intent, contracts, and decisions. Scope: `docs/design/*` (condense),
`README.md` + `CLAUDE.md` (accuracy fixes). `docs/plans/` is historical and
untouched.

## Accuracy fixes found in the reading pass

- `03_cli.md`: doctor exit claim (now exit 0 with findings), list synopsis
  missing `--sort/--reverse/--limit`, config keys missing read-only `last_id`,
  cleanup `dry_run` JSON key, single-mutation plain-row output.
- `02_board-data-model.md`: id allocation now `max(scan, last_id) + 1` — ids
  are never reused, unconditionally.
- `04_skills.md`: `skill check` exit codes (outdated = conflict/4, nothing
  installed = not_found/3, not "exits 1").
- `05_configuration.md`: `last_id` key (read-only, comment-preserving bump).
- `README.md`: list `--limit`, config `last_id`.
- `CLAUDE.md`: status is a fixed value set with free movement, not "freeform".

## Condensing

- `99_roadmap.md`: shipped items become a one-line-each ledger; deferred and
  rejected entries keep the decision + reason, drop the narration.
- `06_testing.md`: the per-file table duplicates the test files themselves —
  reduce to strategy + gate.
- `02/03/05`: trim repetition between files; keep contracts and event matrix.
- `01_overview.md`, `00_index.md`: already tight; touch only if drifted.

## Todo

1. [x] Accuracy fixes across 02/03/04/05, README, CLAUDE.md.
2. [x] Condense 99_roadmap.md (one-line shipped ledger).
3. [x] Condense 06_testing.md and trim 02/03/05.
4. [x] Cross-check final docs against `--help`/behaviour spot-checks.

## Change history

- [2026-07-14] Plan created; pass executed same day.
- [2026-07-14] README condensed 250 → 189 lines: TUI keybinding manual
  reduced to an overview + "press ?" (the in-app help is the reference),
  Dependencies to one line per enforcement level, CLI table trimmed
  (completion/version rows folded into prose, skill rows merged) with the
  contract paragraph pointing at 03_cli.md, repo-layout tree replaced by a
  two-line note under Development, Requirements merged into Install. The
  `NO_COLOR` claim was verified live (zero SGR sequences in a NO_COLOR=1
  TUI capture) and kept.
