---
type: feature
stable_id: F-6
slug: cli-dispatcher
status: implemented
targets: [REQ-1]
paths:
  - cmd/memex/main.go
---

# CLI dispatcher with the upstream subcommand surface

`cmd/memex/main.go` is a hand-rolled subcommand dispatcher (no
`cobra`/`urfave` — just `flag.NewFlagSet` per command, ~325 lines
total).

## Subcommands

| Command | Purpose |
|---|---|
| `search [query]` | Keyword search; supports `--limit`, `--category`, `--tag`, `--author`, `--since`, `--before`. |
| `read <slug>` | Print a card. |
| `write <slug>` | Write a card from stdin; validates required fields (`title`, `created`, `source`); auto-sets `modified`. |
| `links [slug]` | Global stats table or per-card outbound/inbound. |
| `backlinks <slug>` | Sorted list of cards linking to `<slug>`. |
| `archive <slug>` | Move to `~/.memex/archive/`. |
| `organize [since]` | Run the organize report; touch `~/.memex/.last-organize`. |
| `sync [--init / on / off / <url>]` | Git sync wrapper. See [[F-5]]. |
| `mcp` | Start the MCP stdio server. See [[F-4]]. |
| `version` | Print version. |

## Conventions

- `MEMEX_HOME` env var overrides `~/.memex` for testing / alternate
  data dirs.
- All write paths call `sync.AutoSync(memexHome())` after mutating
  the store, matching the JS reference's behavior.
- Search output format matches the JS reference's `formatSearchResult`
  (including the `匹配行: ` line — yes, Chinese — for the matched-line
  preview, deliberately preserved for output parity).

## Not implemented

`serve`, `import`, `doctor`, `migrate` from upstream's CLI — see
[[ADR-0005]] for rationale on the first two and the omitted-features
discussion.
