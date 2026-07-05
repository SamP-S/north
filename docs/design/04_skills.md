# 4. Agent skill

North has no server. AI integration is a single **skill file** that teaches an
agent how to drive the `north` CLI. The skill is embedded in the binary and
installed into the user's agent skill directories on demand.

```bash
north skill install            # project: ./.claude/skills + ./.opencode/skills
north skill install --global   # home dir: ~/.claude/skills + ~/.config/opencode/skills
north skill show               # print the embedded SKILL.md
north skill check [--global]   # compare installed version stamps against the binary
```

- One skill named `north` (`<dir>/north/SKILL.md`).
- Frontmatter declares `name`, a `description` of trigger keywords, and
  `allowed-tools: Bash(north *)`.
- A `<!-- north-skill-version: X -->` comment is stamped into each installed
  file; `north skill check` reads it and reports up-to-date / outdated /
  missing per target (non-zero exit when anything is stale).
- Targets: **Claude Code** (`.claude/skills`) and **opencode** (`.opencode/skills`
  project, `~/.config/opencode/skills` global). opencode also natively reads
  `.claude/skills`, so the Claude install covers it too; we write both for clarity.

The skill body covers the two-axis model (freeform state + status moves), the
command surface, a typical agent work loop (list ready → view → move
in_progress → append results → move done/failed/blocked), the
`--append-body`-vs-`--body` distinction, and the `--plain`/`--json` output and
error contract. `north init` does **not** write an `AGENTS.md` — the skill is
the single source of agent guidance.
