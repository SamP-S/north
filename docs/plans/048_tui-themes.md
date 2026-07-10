# 048 — TUI themes: preset selection via user-level config

## Context

Decided during the deferred-review resumption (2026-07-09). The roadmap's
"TUI themes/config (`tui:` block)" entry is settled as: **no per-slot color
config, no theme downloads — three built-in presets** selected from a
**user-level** config file, because a theme is personal preference and
`north/config.yml` is board policy committed to the shared repo.

- **default** — inherit the terminal's palette: ANSI 0–15 only (plus
  `lipgloss.AdaptiveColor` where light/dark backgrounds need different
  picks). The user's terminal theme *is* the theme. This finishes the
  half-done ANSI migration: today four colors are fixed 256-color values
  (`214` orange, `63` purple-blue, `240`/`238` greys) that ignore the
  terminal scheme.
- **saturated** — a fixed vivid palette (hex; termenv auto-degrades to 256
  colors on non-truecolor terminals). Terminal-independent: for users whose
  terminal scheme is drab or renders the inherited palette poorly.
  Approximately today's look, tuned richer.
- **high-contrast** — ANSI brights only; no dim greys anywhere (dim
  elements use normal foreground or bold instead). For low-contrast
  terminal schemes and accessibility.

No `mono` preset (decided against); `NO_COLOR`/dumb terminals are already
handled by termenv's automatic degradation.

## Behaviour

- User config lives at **`~/.north/config.yml`**:

  ```yaml
  # north user settings (per-user, not per-board)
  tui:
    # theme: default | saturated | high-contrast
    theme: default
  ```

- On `north tui` startup:
  - file missing → **create it** with the commented scaffold above
    (`theme: default`); this is the discoverability story — users find the
    valid values in the file itself.
  - file present → read it; unknown/invalid theme value → **fall back to
    default and surface a yellow warn notice** in the TUI status bar
    (`unknown theme "foo" in ~/.north/config.yml, using default`), same
    channel as all other TUI warnings.
  - unreadable/malformed YAML → same fallback + warning (never block the
    TUI on a bad prefs file).
- Edge decisions (2026-07-09):
  - **Scaffold failure never blocks**: if `~/.north` or the file can't be
    created (read-only home, no `$HOME` in CI), proceed with the default
    theme and a warn notice — the TUI always opens.
  - **Ensure creates, never repairs**: an existing file missing the `tui:`
    block (or the `theme:` key) reads as default silently; North never
    rewrites or re-adds keys to a file the user owns.
  - **Strict names**: exact lowercase `default | saturated |
    high-contrast` (whitespace-trimmed); no aliases — the scaffold comment
    documents the set, and loose matching would blur the warning.
  - **No `NORTH_THEME` env override** (rejected for now): one config
    source; the file is trivial to edit, an env var invites
    forgotten-export drift. Revisit only if theme smoke-testing proves
    painful.
- The CLI is unaffected: `north config get/set/list` stays board-scoped
  (`north/config.yml` only). The user file is edited by hand; the scaffold
  comment documents the options. (A `--user` scope for `north config` can
  be added later if demand shows; not in this plan.)

## Design

### Theme plumbing (`internal/tui/styles.go`)

Replace the flat package-level style vars with one package-level theme
struct instance:

```go
type theme struct {
    ColumnActive, ColumnInactive lipgloss.Style
    CardSelected, CardNormal     lipgloss.Style
    ID, Header, Footer           lipgloss.Style
    // ... one field per existing styleX var
}

var th = defaultTheme() // all render code references th.X

// setTheme installs the named preset; returns "" or a warning.
func setTheme(name string) (warning string)
```

References across `board.go`, `list.go`, `modal.go`, `deps.go` change
mechanically from `styleCardSelected` → `th.CardSelected`. The
`statusStyle`/`stateStyle`/`noticeStyle` helpers stay, reading from `th`.
Status colors inside the theme are a `map[string]lipgloss.Style` keyed by
status name so custom-status boards can extend it later without schema
change.

Palette notes for `default` (ANSI-16 mapping of the four fixed values):
blocked `214` → ANSI 13 (bright magenta — 16-color has no orange; keeps
blocked distinct from in-progress yellow and failed red); active border
`63` → ANSI 5 (magenta, distinct from status colors); dim `240` and
inactive border `238` → ANSI 8 (bright black) — with an
`AdaptiveColor{Light: "245", Dark: "8"}`-style split only if ANSI 8 proves
unreadable on light schemes during smoke testing.

### User config loader (`internal/board/userconfig.go`)

The `board` package already owns YAML config parsing (`LoadConfig`/
`WriteConfig`); mirror that shape:

```go
type UserConfig struct {
    TUI struct {
        Theme string `yaml:"theme"`
    } `yaml:"tui"`
}

// LoadUserConfig reads dir/config.yml; missing file → defaults, no error.
func LoadUserConfig(dir string) (UserConfig, error)
// EnsureUserConfig scaffolds dir/config.yml if absent (commented template).
func EnsureUserConfig(dir string) error
// UserConfigDir returns ~/.north (os.UserHomeDir), overridable in tests
// by passing dir explicitly to the two funcs above.
func UserConfigDir() (string, error)
```

Unknown keys are preserved-by-ignore (same tolerance as `LoadConfig`);
malformed YAML returns the error for the caller to convert into the TUI
warning. Scaffold write uses the existing atomic temp+rename helper if one
is exported, else plain `os.WriteFile` (0644) — it's a prefs file, not
board data.

### Wiring (`internal/cli/tui.go`, `internal/tui/model.go`)

`newTuiCmd`: resolve `UserConfigDir()` → `EnsureUserConfig` →
`LoadUserConfig` → pass the theme name into `tui.NewModel` (new
`tui.Options{Theme string, ThemeWarning string}` or extra param — match
NewModel's current signature style). `NewModel` calls `setTheme`; a
non-empty warning (from setTheme or a load error) becomes the initial
status-bar notice at warn level. Config-dir resolution failure (no home
dir) silently uses default theme — never blocks the TUI.

## Files to modify

- `internal/tui/styles.go` — theme struct, three presets, `setTheme`.
- `internal/tui/model.go` (+ mechanical var→`th.` renames in `board.go`,
  `list.go`, `modal.go`, `deps.go`, `actions.go`) — startup warning notice.
- `internal/board/userconfig.go` (new) — user config load/scaffold.
- `internal/cli/tui.go` — ensure/load user config, pass theme + warning.
- `internal/board/board_test.go` or new `userconfig_test.go` — loader tests.
- `internal/tui/tui_test.go` — theme + warning behavioural tests.
- `README.md` — themes section (three presets, `~/.north/config.yml`).
- `docs/design/03_cli.md` (TUI section) + `docs/design/05_configuration.md`
  — user-level config concept + `tui.theme` key.
- `docs/design/99_roadmap.md` — prune the themes bullet, record the
  decision in the done list.
- `docs/plans/047_deferred-review.md` — record the themes decision in the
  review log.

## Todo

1. [x] Copy this plan to `docs/plans/048_tui-themes.md`.
2. [x] `internal/board/userconfig.go` + tests — UserConfig, LoadUserConfig,
   EnsureUserConfig (scaffold with commented options), UserConfigDir.
3. [x] `internal/tui/styles.go` — theme struct, `default` (finish ANSI-16
   migration), `saturated`, `high-contrast` presets, `setTheme`; mechanical
   `styleX` → `th.X` rename across the TUI package.
4. [x] Wire `internal/cli/tui.go` → `tui.NewModel`; invalid/unreadable
   config → default + startup warn notice; tests.
5. [x] Docs — README, design 03/05, roadmap prune + done entry, 047 log.
6. [x] Gate — `make vet && make test`; manual smoke of all three themes
   (incl. a light-background terminal for the ANSI 8 dim check).
   Automated gate + pty smoke done; the light-background eyeball of ANSI 8
   dim remains a user check.

## Verification

- `go test ./...` — new loader tests (temp dir as config dir: missing file
  scaffolds, valid file reads, invalid theme value surfaces, malformed YAML
  errors) and TUI tests (setTheme presets apply; invalid theme name
  produces the warn notice text).
- Live: `make build`, run `north tui` in a scratch board with (a) no
  `~/.north` → file created, default theme; (b) `theme: saturated` and
  `theme: high-contrast` → visibly distinct; (c) `theme: nonsense` → board
  renders in default with the yellow status-bar warning.

## Change history

- [2026-07-09] Plan opened: three presets (default/saturated/high-contrast,
  no mono), user-level `~/.north/config.yml`, scaffold-on-first-run,
  invalid theme → default + TUI warning. CLI `config` command stays
  board-scoped.
- [2026-07-09] Edge decisions recorded: scaffold failure warns instead of
  blocking; Ensure never rewrites an existing file; strict lowercase theme
  names; `NORTH_THEME` env override rejected for now. Implementation
  started (board userconfig + TUI theme refactor in parallel).
- [2026-07-09] Code landed (todos 1–4): `internal/board/userconfig.go` +
  tests; theme struct with a shared `buildTheme(palette)` so presets differ
  only in colors (high-contrast uses `lipgloss.NoColor{}` for former dim
  slots); `Options{Theme, ThemeWarning, ConfigPath}` on NewModel; CLI
  `tuiOptions()` resolves the user config warn-don't-block and shortens the
  warning path to `~/…`. Gate green; pty-driven live smoke verified
  scaffold-on-first-run, file preservation, the exact
  `unknown theme "nonsense" in ~/.north/config.yml, using default` status
  bar warning, ANSI-only output for default, and truecolor codes for
  saturated. Docs pass delegated.
- [2026-07-09] Docs landed (todos 5–6): README Themes subsection, 03_cli TUI
  config-resolution section, 05_configuration user-level config section
  (State section now notes the one exception to "no global state"), roadmap
  themes bullet pruned + done entry, 047 log entry; keyboard-only stance
  preserved as an explicit Rejected entry (mouse support). Final gate green.
  Outstanding: eyeball the `default` theme's ANSI 8 dim on a
  light-background terminal (swap to an AdaptiveColor split if unreadable).
- [2026-07-10] Deep-dive verification (pty captures of a fully populated
  board across all three themes, per-element SGR audit, HTML render on dark
  + light palettes): default's ANSI 8 dim confirmed readable on common
  light palettes — no change needed. One bug found and fixed: high-contrast
  used ANSI 15 for the active border/help keys (invisible on light
  backgrounds) → now `AdaptiveColor{Light: "0", Dark: "15"}`. Recorded in
  docs that glamour styles the detail pane's task bodies independently of
  the theme (deliberate). Saturated's `#6b7280` rounds to `107;113;128` in
  termenv — cosmetic, ignored.
