# 3. CLI

`north <command>`. Every command (except `init`) finds the board by walking up
from the current directory. Failures print `error [<code>]: <message>` to
stderr; when the failing command was invoked with `--json`, the error is
emitted instead as `{"error":{"code":"…","message":"…"}}` so agents can parse
it (codes: `not_found`, `conflict`, `invalid`, `internal`).

Exit codes follow one contract in every output mode, mapped from the typed
error codes: **0** success, **1** internal, **2** invalid/usage (including
flag/argument mistakes), **3** not_found, **4** conflict. A partially failed
batch exits with the failure code shared by every failed id (1 when the codes
differ).

Mutating commands take a brief advisory file lock (`north/.lock`) so
concurrent `north` processes serialise; a stale lock (crashed holder) is
stolen, and a still-held one is a `conflict` after a short retry budget.

Mutations can succeed **with warnings** (dependency advisories graded by the
board's `deps_enforcement` level — see
[02_board-data-model.md](02_board-data-model.md)): human/plain modes print
`warning: …` to stderr; `--json` mutation payloads carry a `"warnings"`
array (batch payloads too).

| Command | Description |
|---|---|
| `north init` | Scaffold `north/` (config with `version: 1`, `task-template.md`, `.gitattributes`, `.gitignore` for `.lock`/`*.tmp`, the `drafts/ tasks/ archive/` folders). Refuses (`conflict`) when a board already exists at or above the cwd. Human output ends with an "Optional next steps" epilogue (skill install, auto_commit) — suppressed under `--plain`/`--json` |
| `north task create <title> [--assignee A] [--labels ...] [--depends-on ...] [--body \| --body-file] [--plain \| --json]` | Create a task (lands in `drafts/`, status `ready`). Bodyless creates fill from `north/task-template.md` (missing/empty template ⇒ blank body) |
| `north task list [--state draft\|active\|archive\|all] [--status S] [--assignee A] [--deps met\|unmet] [--search TEXT] [--label L] [--sort id\|updated\|title\|assignee] [--reverse] [-l/--limit N] [--plain \| --json]` | List tasks (default: active). `--search` matches id/title/assignee/labels/body (case-insensitive); `--label` is exact (repeatable); `--assignee` is case-insensitive (`--assignee ""` matches unassigned); `--deps met|unmet` filters on dependency resolution (resolved = done or archived); sort default: id, newest first; `--limit N` caps rows after filter+sort (0 = all) |
| `north task view <id> [--plain \| --json]` | Show one task (state, status, fields + body) |
| `north task edit <id> [--title --assignee --labels --depends-on --body \| --body-file \| --append-body] [--plain \| --json]` | Edit fields/body (bumps `updated_at`). `--append-body` appends with a blank-line separator and is exclusive with `--body`/`--body-file` |
| `north task move <id[,id…]> <status> [--plain \| --json]` | Set status, in place (any → any, in any state) |
| `north task state <id[,id…]> <draft\|active\|archive> [--plain \| --json]` | Move tasks between lifecycle folders, preserving status (any → any) |
| `north task delete <id[,id…]> [-y/--yes] [--plain \| --json]` | Delete tasks. With `--plain`/`--json`, non-TTY stdin, or a batch, `-y` is required (no prompt) |
| `north next [-l/--limit N] [--label L] [--plain \| --json]` | Show the next **workable** task (active, `ready`, unassigned, deps met, lowest id; `--label` narrows, repeatable). Pure read. No workable task is a normal outcome: exit 0, `{"task": null}` under `--json`, empty output under `--plain`. `--limit N` (≥ 2) shows the next N in take order, rendered as a task list (`{"tasks": […]}` under `--json`); `--limit < 1` is `invalid` |
| `north take [id] [--assignee A] [--label L] [--plain \| --json]` | Atomically claim the next workable task: same pick as `next`, then `status=in_progress` + `assignee` in **one write under one lock hold**, so concurrent takes get different tasks. With `id`, claim that specific task instead — refused (`conflict`, naming the reason) unless it is active + `ready` + unassigned + deps met (no steal, no overrides; unknown id is `not_found`; `--label` with an id is `invalid`). Assignee falls back to `$NORTH_AGENT`; neither set is `invalid`. Refuses (`conflict`) when the assignee already holds `max_wip` active `in_progress` tasks (`max_wip > 0`). Same empty-result contract as `next` (queue mode only) |
| `north board [--plain \| --json]` | Active counts per status + draft/archive tally |
| `north cleanup [--older-than DAYS] [--dry-run] [--plain \| --json]` | Archive active `done` tasks. The board lock is held for the whole run (snapshot + every archive), so a concurrent status change can never be archived stale. `--dry-run` lists what would be archived without locking or writing; `--json` payloads carry `"dry_run": true\|false` |
| `north doctor [--fix] [--plain \| --json]` | Board integrity check. Exits 0 whenever the scan completes — issues found are the report, not a failure (gate on the `--json` issues array) |
| `north config list \| get <key> \| set <key> <value>` | Read/write `north/config.yml` settings (`auto_commit`, `deps_enforcement`, `max_wip`; `version` and `last_id` are read-only — `set` refuses them) |
| `north skill install [--global]` | Install the agent skill (Claude Code + opencode) |
| `north skill show` | Print the embedded skill |
| `north skill check [--global]` | Compare installed skill version stamps against the binary |
| `north tui` | Interactive terminal UI (human use only) |
| `north completion <shell>` | Generate shell completions (bash/zsh/fish/powershell) |
| `north version` | Print the version (also `north --version`) |

Task ids are bare numbers: `north task view 12`. `move`, `state`, and
`delete` also take a comma-delimited batch (`north task move 2,3,4 done`):
ids are deduplicated, every id is attempted (continue-on-error) with a per-id
report (successes to stdout, `error [<code>]: …` lines to stderr; under
`--json` one `{"tasks":[…],"errors":[…]}` payload), and any failure yields a
non-zero exit. `--body-file -` reads the body from stdin (on `create` and
`edit`).

## Multi-agent work-picking: `next` / `take`
`take` exists because the two-call claim (`list --status ready` then
`move <id> in_progress`) has a TOCTOU race: the lock makes each mutation
atomic on its own but does not span the read-decide-write, and `move` to an
unchanged status is a silent no-op — two agents can both believe they claimed
the same task. `take` selects and claims under a single lock hold, so
concurrent takes hand out different tasks. There is no claim data model:
`assignee` + `status: in_progress` *is* the claim, and it never expires — a
crashed agent's task stays claimed until a human/orchestrator resets it
(`north task move <id> ready` **and** `edit --assignee ""` — a ready task
that keeps its assignee is invisible to `next`/`take`, so `move` to `ready`
warns while an assignee is set). Live coordination requires all agents to
share **one physical `north/` directory** (one checkout); across git
worktrees each checkout has its own board copy, so partition work up front
(e.g. by `--label`) and reconcile at merge (`doctor --fix` heals duplicate
ids). See `docs/plans/049_multi-agent-usage-review.md` for the full analysis.

## State vs. status
`move` changes **status** (the workflow column) — in any state, with a
stderr note when the task is not active (status only shows on the board once
active). `state` changes **state** (the lifecycle folder) and preserves
status. Both are freeform — any value to any other value in one call. See
[02_board-data-model.md](02_board-data-model.md).

## TUI
`north tui` is documented in the [README](../../README.md#tui); the theme
config (user-level `~/.north/config.yml`, three presets, never-blocks
fallback) is specified in [05_configuration.md](05_configuration.md). One
detail that lives only here: the theme colors the **chrome** only — task
bodies in the list view's detail pane are rendered by glamour, which applies
its own light/dark-adaptive document styles independent of `tui.theme` (by
design, not an oversight).

## Output modes
`--plain` (stable, line/tab-oriented text for scripts) and `--json` (the
`Task` dict, or board/list summaries where applicable) are supported uniformly
across the CLI. The default is human-readable. `task list --plain` columns
are `id  state  status  assignee  labels  title` (tab-separated;
assignee/labels empty when unset, labels comma-joined, title last). Mutations
(`create`/`edit`/`move`/`state`/`delete`, and `next`/`take`) print the same
single row per task under `--plain` — only `view` shows the multi-line detail
record — and an empty plain list/pick prints nothing. List/board `--json`
payloads carry a `"warnings"` array naming unparseable task files; in
human/plain modes those warnings go to stderr. For `--labels`/`--depends-on`
on `edit`, passing the flag with no values clears the field; omitting it
leaves it unchanged.

## Prompts
The only interactive prompt is `task delete` without `-y`, and it goes to
stderr. In machine modes (`--plain`/`--json`) or when stdin is not a terminal,
the prompt is replaced by an `invalid` error demanding `-y`.
