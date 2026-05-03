---
type: adr
stable_id: ADR-0005
slug: omit-semantic-search-web-ui-and-importers
status: accepted
relates_to: [REQ-2]
---

# Deliberately omit semantic search, web UI, and openclaw importer

## Context

Three large surfaces of the upstream JS reference are not needed by
this user and account for most of the supply-chain surface area
([[REQ-2]]):

- **Semantic search** (`lib/embeddings.js`, ~492 lines) — three
  embedding providers (OpenAI, Ollama, `node-llama-cpp`), an
  embedding cache, hybrid scoring. Pulls in `node-llama-cpp` as a
  native optional dep.
- **Web UI** (`commands/serve.js` + `serve-ui.html`, ~220 lines) —
  local HTTP server for the timeline view, plus a redirect to the
  hosted `memra.vercel.app` UI when sync is configured.
- **OpenClaw importer** (`importers/openclaw.js`, ~114 lines).

In actual usage, the user relies on:
- Keyword search via the curated `index` card (`memex_recall` reads
  `index` first; semantic search is never invoked from the auto-recall
  path).
- `cat`, `grep`, `vim`, and Obsidian on `~/.memex/cards/*.md` directly.
- No openclaw history.

## Decision

Omit all three from the initial Go port. The keyword-search-only
implementation in [[F-2]] handles `memex_search` and the search
backing for `memex_recall`. The MCP `memex_search` tool keeps its
`semantic` parameter in its schema (for upstream-CLI parity) but
ignores it with a description note: "NOT IMPLEMENTED in this build —
keyword search is used."

## Consequences

- Drops ~830 lines of upstream JS plus the optional `node-llama-cpp`
  native dep — most of the supply-chain win in [[REQ-2]].
- `memex serve` is unimplemented; running it currently fails with
  "unknown command." A future spec session can add it back as ~200
  lines of Go using the existing serve-ui.html — but with the
  `memra.vercel.app` redirect dropped (see the
  `memex-serve-redirects-to-hosted-memra-when-synced` memex card).
- `--semantic` from CLI / MCP silently degrades to keyword search.
  This is a behavior diff from upstream that a future user / agent
  might miss. Future ADR if anyone re-adds semantic should explicitly
  recommend Ollama (HTTP-only, no native deps) over OpenAI or
  llama.cpp.
- `memex import` is unimplemented; openclaw users would notice
  immediately. Not a regression for this user.
