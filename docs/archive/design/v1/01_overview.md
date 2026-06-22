## 1. Overview

### 1.1 Purpose

A self-hosted, single-operator, multi-project agentic development system. It maintains a Git-backed, Markdown-based project board, and orchestrates AI agents (via the Claude Agent SDK) to execute development tasks against project repositories. It runs persistently on a Linux host.

### 1.2 Intent / north star

Enable structured, well-documented remote development while **minimizing cloud usage against a limited monthly credit**. Cloud (Claude) does the expensive cognitive work — planning, architecture, spec-writing, and decomposing features into small, tightly-scoped tasks. Local 7B models execute those small tasks for free. Confidence-based escalation returns a task to cloud only when a local model cannot deliver after bounded retries. Every choice trades toward "as much local execution as possible," conserving the Pro plan's $20/month Agent SDK credit for planning/design and genuinely hard work.

### 1.3 Goals

- A board (Project → Epic → Feature → Task) derived from Git, editable directly on the filesystem.
- **Self-describing projects:** each repo carries its context under `docs/ctx/`; the board lives in the board repo and global agent/pipeline definitions live in aurora. Cloning aurora and the board repo reconstructs the full system state.
- Autonomous task execution with a strict, auditable control flow; agentic workflows defined by composable pipeline graphs with confidence routing, binary checks, and explicit escalation paths.
- Layered cloud-usage control: per-call budget + turn caps, monthly-credit awareness, rate-limit-event handling, and a hard stop before any paid overage.
- A CLI for monitoring queue state and live agent sessions, with pause/resume control and project registration.

### 1.4 Governing principles

- Git is the source of truth; the board is a derived view.
- Agents are stateless; one job per agent, no decision-making authority.
- Humans always resolve conflicts.
- Local by default, cloud by exception.
- Commits run under the operator's Git identity; agent identity lives in the commit-message prefix only.
- Markdown is the interface.
- **Project context is self-contained** — `docs/ctx/` lives in the project repo and travels with a clone; the board lives in the board repo; global agent/pipeline definitions live in aurora.
- **Project repos carry only code, `CLAUDE.md`, and context** — no board files, no agent definitions, no `.claude/` scaffolding.
- **Main changes only via merges** — feature work never edits main directly.
- No Docker — systemd only.
