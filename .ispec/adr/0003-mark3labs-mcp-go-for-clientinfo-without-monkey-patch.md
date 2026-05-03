---
type: adr
stable_id: ADR-0003
slug: mark3labs-mcp-go-for-clientinfo-without-monkey-patch
status: accepted
relates_to: [REQ-1]
---

# Use mark3labs/mcp-go and read clientInfo via SessionWithClientInfo

## Context

The MCP server stamps each `memex_retro` / `memex_write` card's `source`
frontmatter field with the calling client's name (`claude-code`,
`cursor-vscode`, `codex`, etc.), normalised to kebab-case. This is a
load-bearing feature: a card's `source` lets the developer see which
agent / surface produced an insight, and lets the search filter
(`--author`) work.

The upstream JS server (`@modelcontextprotocol/sdk`) does NOT expose
`clientInfo` to tool handlers, so the JS reference monkey-patches
`server.connect()` to wrap `transport.onmessage` and peek at the raw
`initialize` JSON-RPC message. Brittle: depends on the SDK's transport
not overwriting the `onmessage` handler in its own `connect()`.

## Decision

Use `github.com/mark3labs/mcp-go` (v0.50.0). It captures the client's
`Implementation{Name, Version}` automatically during the `initialize`
handshake and exposes it via the `SessionWithClientInfo` interface,
accessible from any tool handler:

```go
session := server.ClientSessionFromContext(ctx)
if withInfo, ok := session.(server.SessionWithClientInfo); ok {
    name := withInfo.GetClientInfo().Name
    // normalize: lowercase + whitespace → "-"
}
```

Default to `"mcp"` when the session is nil or doesn't satisfy the
interface (test transports, weird clients).

## Consequences

- The fragile transport-tap from the JS reference is gone. Future
  upgrades of `mcp-go` won't silently break source-tagging — the
  accessor is part of the library's documented API.
- Direct dep on `mark3labs/mcp-go`, with several transitive deps
  (`google/jsonschema-go`, `spf13/cast`, `yosida95/uritemplate`,
  `google/uuid`, `santhosh-tekuri/jsonschema`). Each is a supply-chain
  surface per [[REQ-2]] and will be re-evaluated on upgrade.
- Library handles the JSON-RPC framing, `tools/list` response shape,
  schema validation of tool inputs — all of which would be ~500 LOC
  to rewrite ourselves.
- Locked into `mcp-go`'s API conventions (`mcp.NewTool`, `WithString`,
  `WithBoolean`, etc.). Migration to a different MCP library would be
  non-trivial.
