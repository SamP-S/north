# 053 — Template section hints + TUI theme preview page

## Summary

1. The default task template (`board.DefaultTaskTemplate`, scaffolded into
   `north/task-template.md` by `north init`) now carries a plain one-line
   description under each section heading, so bodyless creates hand the writer
   guidance instead of bare headings.
2. A disposable smoke-test page, `docs/tui-theme-preview/index.html`, shows
   live tmux captures (120×35) of `north tui` on the populated dogfood board
   for each `~/.north/config.yml` theme preset (default, saturated,
   high-contrast), converted from ANSI to inline-styled HTML.

## Files modified

- `internal/board/board.go` — section hint lines in `DefaultTaskTemplate`.
- `north/task-template.md` — dogfood board template synced to the new default.
- `docs/tui-theme-preview/index.html` — generated preview page (delete when
  no longer needed; regenerate by re-capturing with tmux).

## Todo

1. [x] Add one-line hints to `DefaultTaskTemplate`; sync dogfood template.
2. [x] Capture `north tui` per theme via tmux with the theme temporarily set
       in `~/.north/config.yml` (original config restored).
3. [x] Convert captures to HTML and assemble the preview page.
4. [x] `gofmt`/`go vet`/`go test` clean; verified a bodyless create receives
       the hinted body.

## Change history

- [2026-07-14] Plan created; both items implemented and verified.
- [2026-07-14] Default theme's inactive column/pane border changed from
  ANSI 8 (bright black — invisible on schemes that map it to the background)
  to ANSI 7 (light grey); default-theme capture in the preview page refreshed.
- [2026-07-14] Default theme's `dim` color (task IDs, footer, counts, archive
  labels) moved from ANSI 8 to ANSI 7 for the same reason — the default
  preset no longer uses bright black anywhere.
