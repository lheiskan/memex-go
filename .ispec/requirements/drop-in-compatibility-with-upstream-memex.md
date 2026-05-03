---
type: requirement
stable_id: REQ-1
slug: drop-in-compatibility-with-upstream-memex
stakeholders: [STK-1]
satisfaction: satisfied
---

# Drop-in compatibility with @touchskyer/memex

The Go binary must replace the upstream npm package
(`@touchskyer/memex` v0.1.27) at the same install path with no other
configuration changes:

- The same `~/.memex/cards/<slug>.md` files keep working unchanged. No
  migration, no re-format, no schema diff.
- The MCP server exposes the same 10 tool names with the same input
  schemas, callable from the same `~/.claude.json` registration.
- The CLI accepts the same subcommand surface that hooks and skills
  invoke (`memex search`, `memex read`, `memex sync`, `memex mcp`, etc.).
- Re-saving a card via the new writer produces byte-identical output to
  what the JS reference would emit, so the user's `~/.memex/.git`
  history doesn't churn when the binary is swapped.

## Rationale

The user has ~200 cards, an existing SessionStart hook, an existing MCP
registration, and a sync remote — all wired against the upstream tool's
data format and command surface. Anything that requires touching that
ecosystem is a non-starter; this rewrite is justified only if it slots
in with zero migration cost.

## Acceptance

- All 200 cards round-trip through `card.ParseFrontmatter` →
  `card.StringifyFrontmatter` with zero byte diffs (see [[F-1]]).
- `memex read`, `memex search`, `memex links`, `memex backlinks` produce
  the same output shape as the JS reference against the same inputs.
- An MCP `initialize` handshake from any client is followed by a
  `tools/list` response containing the 10 expected tools (see [[F-4]]).
- A `memex_retro` write through MCP stamps the `source` field correctly
  from `clientInfo.name` and produces a card byte-identical to what
  the JS server would write.

## Status

Satisfied. Round-trip verified on all 200 of the user's real cards;
read-only CLI smoke tests match upstream output; MCP write test
captured `clientInfo.name = "My Test Client"` and produced
`source: my-test-client` correctly.
