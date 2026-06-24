## 1. Overview

### 1.1 Purpose

A self-hosted, single-operator, multi-project task board. It maintains a
Git-backed, Markdown-based project board and exposes it over a REST API and an
MCP surface. It runs persistently on a Linux host as a systemd user service.

### 1.2 Intent

Give structured, well-documented, auditable project state a single home: a board
derived from Git, editable directly on the filesystem and over a typed API. North
owns the board and its lifecycle rules; task execution is delegated to external
clients (the operator, an agent runtime, or any MCP client), which read the queue
and write board state through the same endpoints.

### 1.3 Goals

- A board (Project → Epic → Feature → Task) derived from Git, editable directly
  on the filesystem.
- Every board mutation is exactly one Git commit through the API, so the board is
  fully auditable and recoverable from its remote.
- A server-enforced lifecycle: a draft gate before work begins, a transition
  table that rejects illegal status jumps, and a feature review gate.
- A typed interface — REST plus an MCP surface mounted in the same process — so
  external clients can drive work without North knowing how tasks are executed.
- A CLI for inspecting and editing board state and managing the service.

### 1.4 Governing principles

- Git is the source of truth; the board is a derived view.
- Humans always resolve conflicts.
- Commits run under the operator's Git identity; the writer's identity lives in
  the commit-message prefix only.
- Markdown is the interface.
- **Project context is self-contained** — `docs/ctx/` lives in the project repo
  and travels with a clone; the board lives in the board repo.
- **Project repos carry only code, `CLAUDE.md`, and context** — no board files.
- **Main changes only via merges** — feature work never edits main directly.
- No Docker — systemd only.
