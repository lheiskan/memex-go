---
type: requirement
stable_id: REQ-2
slug: audit-supply-chain-risk-surface
stakeholders: [STK-1]
satisfaction: satisfied
---

# Auditable supply-chain risk surface for an always-on MCP server

The memex binary runs as a long-lived MCP subprocess of every Claude
Code session, with read/write access to `~/.memex/` and shell-out to
`git`/`gh`. The user must be able to read, in full, the code that
runs with those privileges, and a future update must not silently
introduce code the user hasn't reviewed.

## Rationale

The upstream npm package is globally installed (`/opt/homebrew/bin/memex`
symlink to `dist/cli.js`), pulls in transitive dependencies including
optional native code (`node-llama-cpp`), and updates by `npm update -g`.
Any maintainer push — or any compromise of the npm publish pipeline —
becomes a code-execution vector on the developer's workstation, with
the privileges listed above. The risk model that motivated the rewrite
was "I cannot personally audit every transitive dependency on every
update."

## Acceptance

- Total Go source under this repo is small enough that the user can
  read it end-to-end before installing (initial size: 1556 lines of
  production code + 417 of tests).
- Direct dependencies are limited to a small, named set
  (`gopkg.in/yaml.v3`, `mark3labs/mcp-go` and its transitives) pinned
  in `go.sum` — every update produces a reviewable diff.
- The build is a single `go build`; no native deps, no postinstall
  scripts, no auto-update mechanism.
- Replacing the binary is a deliberate `cp` over the existing path,
  not an automatic process.

## Status

Satisfied for the initial implementation. Future spec sessions that
add dependencies must consider this requirement explicitly — the
risk surface is monotonically growing.
