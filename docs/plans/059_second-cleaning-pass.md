# 059 — second cleaning pass

## Summary

Follow-up audit after the 057/058 robustness pass (commit 59a0db0): full re-read of every file the fixes touched plus the corners the first pass skipped (`tui/styles.go`, `Makefile`, `go.mod`, CI workflows). Build/vet/tests green at audit time. Findings are smaller than round one — mostly consistency gaps the 058 fixes themselves introduced or made visible.

## Findings

### A. Robustness

1. **Config still read before the lock in four mutations.** 058-A5 moved `Take`'s config read under the lock, but `Create`, `Edit`, `SetStatus`, and `Delete` still call `depsPolicy`/`LoadConfig` *before* `board.Lock` — a concurrent `config set deps_enforcement` (which now correctly takes the lock) can be enforced stale by one op. Same class of bug, same fix: load config after acquiring the lock in all four.

2. **`stealStale`'s restore path breaks on filesystems without hard links.** The fresh-lock restore uses `os.Link` for its no-clobber semantics, then unconditionally `os.Remove(stolen)`. On FAT/exFAT/some network mounts `link(2)` fails with ENOTSUP — the restore silently fails and the remove then destroys a *live* lock. Fix: inspect the link error — on EEXIST proceed to remove (a newer lock exists, drop ours); on any other failure fall back to renaming the stolen file back into place (accepting the tiny clobber window only on link-less filesystems) and skip the remove when the rename also fails.

3. **`config set` echoes the raw input, not the canonical value.** Human mode prints `auto_commit: TRUE` while `true` was stored; plain/JSON print the canonical value but obtain it by re-reading config.yml from disk (redundant I/O, and a second failure point after the write already succeeded). Fix: print the canonical value in every mode, no re-read.

4. **`config set`'s under-lock `LoadConfig` has a misleading comment.** The result is discarded — the call is a malformed-YAML gate (correct and worth keeping), but the comment claims it prevents overwriting concurrent changes, which is actually `SetConfigValue`'s node round-trip. Fix the comment.

5. **`--body` + `--body-file` together is silently resolved.** `readBody` prefers `--body-file` and ignores `--body` without complaint, while the analogous `--append-body` + `--body` combination is rejected as invalid. Reject the pair the same way.

6. **`take <id> --label " "` errors as a combination that isn't there.** The "cannot be combined with an explicit id" check runs on the raw flag values; a whitespace-only label (which `cleanLabelFilter` would drop) still triggers it. Clean first, then check — matching how `next` treats whitespace labels.

### B. Dead code / hygiene

7. **`board.WriteConfig` has no production callers.** After 058-A1 routed `config set` through `SetConfigValue`, only tests call it. Remove it and migrate the six test call sites (scaffold via `InitBoard`/`SetConfigValue` instead) — dead exported API on the config-write path is exactly where the next comment-clobbering bug would come from.

8. **CI action version drift.** `release.yml` pins `actions/checkout@v4` / `setup-go@v5`; `ci.yml` uses `@v5` / `@v6`. Align release.yml to ci.yml's versions.

### C. Checked and cleared

- The 058 fixes themselves: lock steal (modulo finding 2), config round-trip write, trimming, JSON envelopes, warnings channels, exit codes, flag aliases — all verified sound on re-read.
- `skill check --json` printing both the payload (stdout) and the error object (stderr) when outdated is the documented contract, not double-reporting.
- `tui/styles.go` (first full read): palette/theme structure is clean, colors-only presets as required.
- Makefile, go.mod tidy; `make vet` includes the gofmt check; release workflow runs vet+test before dist.
- `configVersion` tolerating a malformed `version:` value (treated as v1) matches `checkVersion`'s documented leniency.

## Files to modify

- `internal/tasks/tasks.go` — 1 (Create/Edit/SetStatus/Delete config-under-lock)
- `internal/board/lock.go` (+ lock tests) — 2
- `internal/cli/config.go` — 3, 4
- `internal/cli/task.go` — 5
- `internal/cli/next.go` — 6
- `internal/board/board.go` + `internal/board/board_test.go`, `internal/cli/cli_test.go` — 7
- `.github/workflows/release.yml` — 8

## Todo

1. [x] Write this plan
2. [x] Implement 1–8 (+tests where behaviour changed: 1, 2, 3, 5, 6)
3. [x] Verify: `make build vet` + `go test ./...` + scratch-board probes (config set echo, body+body-file rejection, take label edge)

## Change history

- [2026-07-19] Audit completed; plan written.
- [2026-07-19] Implemented findings 1–8: config-under-lock in Create/Edit/SetStatus/Delete; stealStale restore hardened (restoreFresh helper: EEXIST tolerated, rename fallback on link-less filesystems, temp kept if both fail); config set echoes canonical value in all modes without re-read + comment fixed; --body/--body-file rejected together in readBody; take cleans labels before the id-combination check; board.WriteConfig deleted and six test call sites migrated to SetConfigValue/os.WriteFile; release.yml bumped to checkout@v5/setup-go@v6. Tests added for the lock EEXIST branch, canonical set echo (3 modes), body flag exclusivity (create+edit), and take id + whitespace-only label. make build vet + go test ./... green.
- [2026-07-19] Verify done: build/vet/tests green; scratch probes — `set auto_commit TRUE` echoes `true` (all modes, `max_wip 03` → typed `3`), `--body`+`--body-file` rejected (exit 2), `take <id> --label " "` claims cleanly, scaffold comments intact post-WriteConfig removal. Plan complete.
