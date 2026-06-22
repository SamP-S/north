## 8. Git conventions and integration

### 8.1 Commit prefixes and targets

The summary's prefix table is authoritative. Agent and system prefixes never trigger reconciliation:

| Prefix | Source | Target repo | Target branch |
|---|---|---|---|
| `[agent:*]` | agents | project repo | feature branch (code) |
| `[system:task]` | runner task-file status/output/log | board repo | board repo main |
| `[board:task]` / `[board:feature]` | operator board edits | board repo | board repo main |
| `[board:epic]` / `[board:project]` | operator epic / project-registry edits | board repo | board repo main |
| *(anything else)* | human | project repo | any branch |

All commits run under the operator's Git identity; the prefix is the only origin marker.

### 8.2 Branch / worktree model

One worktree per active feature at `$AURORA_HOME/worktrees/{project}/{feature}` (created on feature detection, removed on merge), holding the **project code only**. Board files are not in the worktree — the runner reads and writes them directly from the board repo. Worktrees share the managed clone's `.git` (`$AURORA_HOME/repos/{project}`). The managed clone's `main` checkout is the base for new feature branches and holds `docs/ctx/`.

**Feature creation:** the operator creates and commits `_feature.md` with a `[board:feature]` prefix. The runner detects the new commit on the next supervisor poll iteration, validates the frontmatter, creates the feature branch off `base_branch` (or adopts it if it already exists), and creates the worktree. `base_branch` is always read from `projects.yaml` for the feature's parent project — it is not stored in the feature file.

**Branch adoption:** if the branch named in `_feature.md` already exists, the runner checks that its merge-base with `base_branch` is `base_branch` HEAD (i.e. it diverged cleanly from base with no unrelated history). If the check passes, the branch is adopted as-is. If it fails — the branch has diverged or has unrelated commits — the feature is set to `status: blocked` and a Telegram notification fires; no worktree is created until the operator resolves the conflict and commits a corrected `_feature.md`. If frontmatter validation fails, the runner logs a warning and fires a Telegram notification; no branch or worktree is created until the operator commits a corrected file.

**Feature close (without merge):** operator sets `status: closed` in `_feature.md` and commits it with `[board:feature]`. The runner detects the commit and performs the full reject sequence — identical to `aurora reject`: resets the feature branch to `base_branch` HEAD, pushes to origin, archives the board (`active/ → archived/`), and removes the worktree. Direct file edit and `aurora reject` are equivalent; there is no "soft close" path.

Minimum required frontmatter for `_feature.md`: `id`, `title`, `branch`, `epic`, `status: open`.

### 8.3 Hooks

A **post-commit hook** in `$AURORA_HOME/repos/{project}/.git/hooks/post-commit` (shared across worktrees) is installed at feature creation. In v1 it is a no-op placeholder; automated reconciliation (tracking human commits and updating the board) is a planned feature — see [planned features](99_planned-features.md).

### 8.4 Feature review and merge flow

**Triggered automatically when all tasks reach `done`:**

1. Runner sets feature `status: review`; commits `[board:feature]` to board repo; fires Telegram "feature ready for review" notification
2. Dependent features remain blocked — the resolver skips any feature whose `depends_on` includes a feature not yet `merged`
3. Operator reviews the diff locally (e.g. `git diff base_branch..feature/xyz` in the managed clone or project worktree)

**Operator then runs one of three CLI commands:**

`aurora approve <feature> [--project <project>]`
1. Aurora merges the feature branch into `base_branch` in the managed clone (`git merge --no-ff`)
2. If merge conflicts: aborts, reports conflicts to the operator; feature remains `review`; operator resolves manually then re-runs `aurora approve`
3. On success: pushes `base_branch` to origin; sets feature `status: merged`; archives board (`active/{feature}/ → archived/{feature}/`); commits `[board:feature]`; removes worktree; fires Telegram "feature merged"

`aurora rollback <feature> [--project <project>]`
1. Prints a warning listing the number and one-line summaries of all commits on the feature branch since it diverged from `base_branch` — including any human commits, not just agent commits — so the operator knows exactly what will be discarded
2. Resets the feature branch to `base_branch` HEAD (`git reset --hard $(git merge-base feature base_branch)`)
3. Pushes the reset branch to origin
4. Resets all tasks in the feature to `status: ready`; sets feature `status: open`; commits `[board:feature]` + task updates to board repo
5. Fires Telegram "feature rolled back — tasks re-queued"; runner picks up the feature again on next loop

Rollback discards all commits on the feature branch — human and agent alike. The pre-rollback warning is the operator's only prompt to recover anything before it is gone.

`aurora reject <feature> [--project <project>]`
1. Resets the feature branch to `base_branch` HEAD; pushes to origin
2. Sets feature `status: closed`; archives board; commits `[board:feature]`; removes worktree
3. Fires Telegram "feature rejected"

**Concurrency safety:** `approve`/`rollback`/`reject` operate on the managed clone (`base_branch` + the target feature ref); the runner operates in a separate worktree on a different feature ref. These share `.git` but touch different indexes and refs — no locking conflict.

### 8.5 Push cadence

The board repo is pushed to its private remote on **every board commit** (`[system:task]`, `[board:*]`) — no batching. This keeps board state continuously backed up; since the runner is single-worker, commit frequency is naturally low and push overhead is negligible. Project feature branches are pushed to their origin at the end of `update_board` after every completed task — no batching, no configuration flag. This ensures completed task code is backed up continuously; if the server dies mid-feature, all completed task work is recoverable from origin. Project main is pushed on context/merge commits.

**Push conflict handling:** if a board repo push fails (non-fast-forward, due to a concurrent operator push), the runner does `git pull --rebase` and retries the push once. If the rebase produces a conflict (both runner and operator edited the same lines in the same file), the runner aborts the rebase (`git rebase --abort`), logs the error, fires a Telegram notification ("board push conflict — manual resolution required"), and continues without pushing. The local commit remains; the operator resolves the conflict manually and pushes.
