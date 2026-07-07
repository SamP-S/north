# 045 — Task sorting (TUI picker + CLI --sort)

Decisions (user, 2026-07-06): default order becomes **id descending** (newest
first) everywhere; sort keys **id / updated / title / assignee**; TUI gets an
`o` picker listing ascending and descending as separate entries (8 items);
CLI `task list` gets `--sort <key>` + `--reverse` (default id descending,
matching the TUI).

## Todo

1. [x] `internal/tasks/sort.go`: `SortKey` type + `ParseSortKey`, `Sort(ts,
   key, desc)` — id numeric; updated nil-safe; title/assignee case-insensitive,
   empty assignee last; id tiebreak. Unit tests.
2. [x] TUI: root-owned sort state (default id desc) pushed to board (rebuild)
   and list (refilter); `o` opens `modalSortPicker` ("id ↓", "id ↑", …);
   selection applies immediately; footer hints + help row.
3. [x] CLI: `task list --sort id|updated|title|assignee [--reverse]`,
   default id desc; validation via ParseSortKey.
4. [x] Docs: README (TUI bullets, CLI table, quick mention), 03_cli.md,
   SKILL.md list synopsis. Drop sortAsc/DescByID in favour of tasks.Sort.
5. [x] Gate: make vet && make test.

## Change history
- [2026-07-06] Plan written.
- [2026-07-06] Implemented in full; gate green.
