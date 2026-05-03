---
type: feature
stable_id: F-4
slug: mcp-server
status: implemented
targets: [REQ-1]
paths:
  - mcp/server.go
---

# MCP stdio server with 10 tools

`memex mcp` runs a JSON-RPC 2.0 stdio server speaking MCP via the
`mark3labs/mcp-go` library (per [[ADR-0003]]). Registered in
`~/.claude.json` so each Claude Code session spawns one MCP server
subprocess.

## Tools

| Tool | Purpose |
|---|---|
| `memex_recall` | Auto-invoked at task start; reads the `index` card or returns a list. Accepts query + filter args. |
| `memex_retro` | Auto-invoked at task end; writes a card with `source` stamped from `clientInfo.name`. |
| `memex_organize` | Network maintenance report (orphans, hubs, conflicts, neighbor pairs). |
| `memex_search` | Low-level keyword search (semantic flag accepted but ignored — see [[ADR-0005]]). |
| `memex_read` | Read a card by slug. |
| `memex_write` | Write a card (full frontmatter + body). Stamps `source` if missing. |
| `memex_links` | Link graph stats (per-card or global). |
| `memex_archive` | Move a card to `~/.memex/archive/`. |
| `memex_pull` | Git pull. Returns "Sync not configured" with setup hint if no remote. |
| `memex_push` | Git push. Same fallback as pull. |

## Hooks

- On connect: `sync.AutoFetch(home)` runs once if a sync remote is
  configured.
- `memex_recall` / `memex_retro` / `memex_organize` each prefix with
  `AutoFetch` and (for retro/organize) suffix with `AutoSync`.
- All sync operations swallow errors silently — sync is infrastructure,
  not business logic. See [[F-5]].

## Source-tagging

`clientNameFromCtx` reads `SessionWithClientInfo.GetClientInfo().Name`,
lowercases, replaces whitespace with `-`. Defaults to `"mcp"`. This is
the fragility-elimination win from [[ADR-0003]] vs the JS upstream's
transport monkey-patch.
