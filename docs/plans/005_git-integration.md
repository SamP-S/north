# 005 — Git Integration

## Summary

Implement the full Git layer: feature detection from board commits, branch creation/adoption, worktree lifecycle, and post-commit hook installation. Completes `detect_git_changes()` with real branch/worktree actions (stubs were wired in 002). Depends on 001 (board parser) and 002 (supervisor loop).

## Files to Create / Modify

```
service/
  git/
    __init__.py
    features.py                # feature detection: branch create/adopt, worktree create/remove
    worktree.py                # worktree lifecycle helpers
    hooks.py                   # post-commit hook install
  orchestrator/
    git_watcher.py             # extend detect_git_changes() with real branch/worktree actions
tests/
  test_git_features.py
  test_worktree.py
```

## Todo

- [x] 1. `service/git/worktree.py` — `create_worktree(managed_clone_path, worktree_path, branch)`: `git worktree add <path> <branch>`; idempotent (no-op if already exists); `remove_worktree(worktree_path)`: `git worktree remove --force`; `reset_worktree(worktree_path, commit_hash)`: `git reset --hard <hash> && git clean -fd`; all operations scoped to `$AURORA_HOME/worktrees/{project}/{feature}`
- [x] 2. `service/git/hooks.py` — `install_post_commit_hook(managed_clone_path)`: write no-op placeholder shell script to `{managed_clone}/.git/hooks/post-commit`; set executable bit; idempotent (overwrite if exists)
- [x] 3. `service/git/features.py` — `create_feature_branch(managed_clone, branch_name, base_branch)`: create branch off `base_branch` HEAD if it does not exist; if branch already exists, run adoption check
- [x] 4. `service/git/features.py` — `adopt_feature_branch(managed_clone, branch_name, base_branch)`: compute `git merge-base branch base_branch`; pass if merge-base equals `base_branch` HEAD (clean divergence); fail if not — log warning, fire Telegram "branch adoption failed — unrelated history"; return `status: blocked`; do not create worktree on failure
- [x] 5. `service/git/features.py` — `setup_feature(feature_state, project_state)`: full feature initialisation sequence: validate frontmatter minimum fields (`id`, `title`, `branch`, `epic`, `status: open`); call `create_feature_branch` or `adopt_feature_branch`; on success: create worktree; install post-commit hook; update in-memory feature state
- [x] 6. `service/git/features.py` — `teardown_feature(feature_state, project_state)`: remove worktree; leave branch in place (branch management handled by approve/reject in 006)
- [x] 7. `service/orchestrator/git_watcher.py` — extend `detect_git_changes()`: on `_feature.md` added/modified: validate frontmatter; call `setup_feature()` for new features with `status: open`; call `teardown_feature()` for `status: closed`; fire Telegram on invalid frontmatter; validate directory name matches `id` field (warn + Telegram on mismatch); update in-memory state
- [x] 8. Integration tests — new `_feature.md` commit detected → branch created + worktree exists; `_feature.md` with `status: closed` → worktree removed; branch adoption: clean divergence passes; unrelated history fails → `status: blocked`; invalid `_feature.md` frontmatter → Telegram fired, no branch/worktree created; directory name ≠ `id` → warning + Telegram
- [x] 9. Run `uv run ruff check .` and `uv run mypy service/` — fix all errors

## Change History

- [2026-06-07] Implemented full git layer: `service/git/{worktree,hooks,features}.py`; extended `service/orchestrator/git_watcher.py` with feature setup/teardown, directory-name validation, and Telegram alerts. Added `service/notifications/telegram.py` (outbound sender with retry, no-op when unconfigured). Added `FeatureStatus.BLOCKED`. Added `tests/test_worktree.py` (7) and `tests/test_git_features.py` (10). Suite: 64 → 81 passing; ruff clean; mypy clean.
