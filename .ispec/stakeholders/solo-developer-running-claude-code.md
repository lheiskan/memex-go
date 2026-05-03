---
type: stakeholder
stable_id: STK-1
slug: solo-developer-running-claude-code
status: active
---

# Solo developer running Claude Code

A single human developer who uses Claude Code (and occasionally other MCP
clients — Cursor, Codex) on a personal workstation, with `memex` registered
as an MCP server and as a CLI in PATH. The same developer maintains
multiple unrelated projects and accrues cross-project knowledge as
markdown cards under `~/.memex/cards/`.

## Jobs to be done

- Have a Zettelkasten knowledge graph that AI agents can recall before
  starting work and write to after finishing — without locking the data
  inside a service or proprietary format.
- Read, edit, grep, and back up cards as plain files.
- Keep the same data accessible from multiple agents (Claude Code on host,
  Claude Code in YOLO containers, occasional iOS via Working Copy +
  Obsidian) without divergence.

## Constraints

- Has security concerns about granting persistent filesystem-and-MCP
  access to a globally-installed third-party Node.js package that
  auto-updates from npm. The supply-chain risk surface drove this
  reimplementation; future trust decisions about the codebase need to
  preserve "I read this commit before installing it."
- Wants the tool to disappear into the workflow. No mandatory cloud
  components, no background services, no required API keys.
- Prefers tools that work the same way on macOS arm64 host and Linux
  arm64/amd64 containers.

## Communication channel

Direct conversation in Claude Code sessions. Issues / decisions land as
commits in this repo or as cards in `~/.memex/`.
