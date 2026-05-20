---
type: feature
stable_id: F-3
slug: link-graph-and-organize
status: implemented
targets: [REQ-1]
paths:
  - graph/graph.go
---

# Link graph: links, backlinks, organize report

Builds a single in-memory link map from a full scan of the card store,
then serves three consumers.

## Surface

- `Build(s) (Maps, error)` — walks every card once, returns
  `Outbound[slug] []string`, `Inbound[slug] []string`, plus per-card
  `Meta{Title, Modified, Status, Body}`.
- `Backlinks(s, target) ([]string, error)` — sorted slugs that link
  to `target` via `[[wikilinks]]`.
- `Organize(s, lastOrganize) (string, error)` — produces the report
  surfaced by `memex_organize` and the `memex organize` CLI:
  - Per-card outbound/inbound stats table.
  - Orphans (zero inbound, excluding the `index` card).
  - Hubs (≥10 inbound).
  - Conflict cards (`status: conflict` in frontmatter).
  - Recently-modified-card ↔ neighbor pairs for contradiction
    review (capped at 20 pairs, with a "and N more" footer).

## Notes

- The `index` card is exempt from orphan detection because it's the
  curated entry point; nothing else links *to* it on purpose.
- The recent-modification window is `lastOrganize` (a YYYY-MM-DD
  string usually read from `~/.memex/.last-organize`). Empty string
  means "first run, all cards are recent."
- The report tool (CLI `organize` and MCP `memex_organize`) does NOT
  write `.last-organize`. That responsibility lives in the
  `memex-organize` skill, which writes it as its final step after
  the index rebuild. This prevents the marker from advancing when an
  agent fetches the report but skips the maintenance work.
