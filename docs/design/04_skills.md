# 4. Agent skill

North has no server. AI integration is a single **skill file** that teaches an
agent how to drive the `north` CLI. The skill is embedded in the binary and
installed into the user's agent skill directories on demand.

```bash
north skill install            # project: ./.claude/skills + ./.opencode/skills
north skill install --global   # home dir: ~/.claude/skills + ~/.config/opencode/skills
north skill show               # print the embedded SKILL.md
```

- One skill named `north` (`<dir>/north/SKILL.md`).
- Frontmatter declares `name`, a `description` of trigger keywords, and
  `allowed-tools: Bash(north *)`.
- A `<!-- north-skill-version: X -->` comment is stamped into each installed file
  so outdated installs can be detected.
- Targets: **Claude Code** (`.claude/skills`) and **opencode** (`.opencode/skills`
  project, `~/.config/opencode/skills` global). opencode also natively reads
  `.claude/skills`, so the Claude install covers it too; we write both for clarity.

The skill body covers the two-axis model (state vs. status), the command surface,
and the `--plain`/`--json` output modes. `north init` does **not** write an
`AGENTS.md` — the skill is the single source of agent guidance.
