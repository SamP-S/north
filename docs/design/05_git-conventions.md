## 5. Git conventions

### 5.1 Commit prefixes and targets

Every board mutation is exactly one commit through the API. The prefix marks the
writer; all commits run under the operator's Git identity.

| Prefix | Source | Target repo | Target branch |
|---|---|---|---|
| `[board:task]` | task status / result / thread updates | board repo | board repo main |
| `[board:feature]` | feature lifecycle updates | board repo | board repo main |
| `[board:project]` | project-registry edits | board repo | board repo main |
| `[board:comment]` | comment-thread appends | board repo | board repo main |
| `[agent:*]` | external runtime | project repo | feature branch (code) |
| *(anything else)* | human | project repo | any branch |

### 5.2 Branch / merge model

Each feature owns a branch in the project repo, named in `_feature.md`. North does
not check out project code or run merges — that is execution-side work owned by an
external runtime or the operator. North records the feature's `branch` and its
lifecycle status; the actual branch/worktree/merge operations happen outside North
against the project repo.

**Feature creation:** a feature is created `draft` (via the API or a direct
`[board:feature]` commit) and promoted to `open`. North validates `_feature.md`
frontmatter on detection; a mismatch between the directory name and the `id` field
logs a warning and fires a notification.

**Merge / review:** when all of a feature's tasks reach `done`, North sets the
feature to `review`. An external actor reviews the diff, merges the feature branch
into `base_branch` (`--no-ff`), and PATCHes the feature to `merged` (board
archived `active/ → archived/`) or `closed` (rejected/abandoned). Dependent
features remain blocked until the prerequisite is `merged`.

### 5.3 Reading feature history

Features merge with `--no-ff`, so `git log --first-parent main` reads as one line
per feature, while stage-granular commits stay reachable for `blame`/`bisect`
(use `git bisect --first-parent` for feature granularity).

### 5.4 Push cadence

The board repo is pushed to its private remote on **every board commit** — no
batching. This keeps board state continuously backed up; commit frequency is
naturally low and push overhead is negligible.

**Push conflict handling:** if a board repo push fails (non-fast-forward, due to a
concurrent operator push), North does `git pull --rebase` and retries the push
once. If the rebase produces a conflict (both North and the operator edited the
same lines in the same file), North aborts the rebase (`git rebase --abort`), logs
the error, fires a notification ("board push conflict — manual resolution
required"), and continues without pushing. The local commit remains; the operator
resolves the conflict manually and pushes.
