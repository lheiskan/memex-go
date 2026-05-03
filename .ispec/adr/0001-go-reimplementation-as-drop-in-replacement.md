---
type: adr
stable_id: ADR-0001
slug: go-reimplementation-as-drop-in-replacement
status: accepted
relates_to: [REQ-1, REQ-2]
---

# Reimplement memex in Go as a drop-in replacement, not a fork or wrapper

## Context

The user has been running `@touchskyer/memex` (Node.js, ~2,600 LOC of
compiled JS) installed globally via npm. Two motivations converged:

1. Supply-chain unease (per [[REQ-2]]): every `npm update -g` swaps
   code in a long-lived MCP subprocess with full filesystem access.
2. Aesthetic / dependency objections to the parts of upstream that pull
   in OpenAI, Vercel, and `node-llama-cpp` — see [[ADR-0005]].

Three options were considered: (a) freeze the npm version and audit each
update by hand; (b) fork the JS upstream, strip the fluff, run from a
local clone; (c) reimplement in Go from scratch.

## Decision

Reimplement in Go, targeting byte-identical compatibility with the
upstream's on-disk format and MCP / CLI surface (per [[REQ-1]]). Keep
the same `~/.memex/` data layout, the same 10 MCP tool names, the
same subcommand names. Distribute as a single static binary at
`/opt/homebrew/bin/memex`, replacing the existing npm symlink.

## Consequences

- The user can audit the entire codebase before installing — initial
  size is ~2,400 LOC of Go (production + tests) vs. ~2,600 LOC of JS
  in the upstream's compiled `dist/` plus its transitive deps.
- No npm, no Node.js runtime requirement, no native build step. A
  single `go build` produces a binary that works on macOS arm64 and
  Linux arm64/amd64.
- Skills and SessionStart hooks (`memex sync`, `memex read index`,
  etc.) keep working because the CLI surface matches; no edits to
  `~/.claude/settings.json` or `~/.claude.json` needed.
- The `~/.memex/cards/` git history doesn't churn because the
  emitter is byte-identical to the JS reference (per [[ADR-0002]]).
- The fork option (b) was rejected because it doesn't actually solve
  the supply-chain problem — Node + transitive deps still need
  auditing on every dependency update of the fork itself.
- The freeze option (a) was rejected because it leaves the OpenAI /
  llama.cpp / Vercel surfaces in the binary, plus closes off bug
  fixes from upstream.
