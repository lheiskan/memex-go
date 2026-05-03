---
type: feature
stable_id: F-2
slug: keyword-search
status: implemented
targets: [REQ-1]
paths:
  - search/search.go
---

# Keyword search with manifest filters

The `search` package implements the search backing for both
`memex search` (CLI) and `memex_search` / `memex_recall` (MCP).
Semantic search is NOT implemented (see [[ADR-0005]]).

## Behavior

- Tokenises the query on whitespace; AND logic — every token must
  appear (case-insensitive) in title + body.
- Ranks by total token-hit count across matching cards.
- With no query: returns every card as a list (slug + title only).
- Optional manifest pre-filter via `Filter{Category, Tag, Author,
  Since, Before}`. Tag filter handles both array form (`tags: [a, b]`)
  and the lossy comma-string form that [[ADR-0002]]'s emitter
  produces; matches the JS reference's `matchesFilter` exactly.

## Known carry-over bug

`dateString` in `search.go` slices the first 10 chars of a frontmatter
date string. For cards with the legacy `created: 'Thu Apr 16 2026
03:00:00 GMT+0300 (...)' ` form, this yields `"Thu Apr 16"` and the
`--since` / `--before` filters silently misbehave. The bug is
preserved deliberately for behavior parity with the JS reference; a
future spec session should fix it (and likely the upstream too).
