---
type: spec
stable_id: SPEC-domain-model
slug: domain-model
audience: [developer]
satisfies_features: [F-1, F-2, F-3, F-4]
paths:
  - card/parser.go
  - card/store.go
---

# Domain model

The whole tool's vocabulary, grounded in the on-disk shape under
`~/.memex/`. The format is fixed by [[REQ-1]] (drop-in compatibility
with the upstream npm package); changing it without a migration is a
breaking decision.

## Storage layout

```
$MEMEX_HOME/                  # default ~/.memex
├── cards/                    # active cards
│   ├── <slug>.md
│   └── <nested/path>.md      # only if nestedSlugs config is on
├── archive/                  # archived cards (same shape, slugs preserved)
├── .sync.json                # sync adapter config (remote, auto, lastSync)
├── .last-organize            # YYYY-MM-DD of last `memex organize`
└── .git/                     # if sync is configured
```

`MEMEX_HOME` env var overrides the default location. The CLI and MCP
server respect it consistently.

## Card

A `<slug>.md` file under `cards/`. Two parts separated by `---` fences.

### Frontmatter

YAML inside the fences. Field order is significant — preserved through
parse / write so re-saving doesn't churn. See [[ADR-0002]] for the
emitter's rules.

| Field | Type | Required? | Notes |
|---|---|---|---|
| `title` | string | yes | Short noun phrase, ≤60 chars by convention. |
| `created` | string (YYYY-MM-DD) | yes | Auto-coerced from `time.Time` to ISO short. |
| `source` | string | yes | Stamped from `clientInfo.name` on MCP writes if absent. |
| `modified` | string (YYYY-MM-DD) | auto-set | Always updated on write. |
| `category` | string | no | Free-form (`frontend`, `architecture`, etc.). |
| `tags` | array or comma-string | no | Both forms accepted on read; emitter converts arrays to comma-string (lossy). |
| `status` | string | no | Special value `conflict` is surfaced by `Organize`. |

### Body

Markdown after the closing `---`. Wikilinks (`[[other-slug]]`) are
extracted by `card.ExtractLinks` and feed the [[F-3]] link graph.

## Slug

Identifier used in URLs / CLI args / wikilinks. Validated by
`card.ValidateSlug`:

- Non-empty after trim.
- Not just dots and slashes.
- No reserved chars (`: * ? " < > |`).
- No empty path segments, no `..` traversal.

In `nestedSlugs` mode, slugs preserve the relative path under
`cards/` (e.g. `adr/0001-foo`). Default mode uses the basename
without extension.

## Source field overloading

The `source` field is overloaded:

- For MCP writes via `memex_write` / `memex_retro`, it carries the
  client name (`claude-code`, `cursor-vscode`, `codex`, `mcp`).
- For agent-authored cards that explicitly set `source: retro` or
  `source: organize` in the frontmatter, it tags the *operation* that
  produced the card.

This is inherited from upstream and not currently fixed.

## Index card

A specially-named card with `slug: index`. Acts as the curated entry
point for `memex_recall` — it's read first when no query is given,
before the system falls back to a flat card list. Conventionally
maintained by the `memex_organize` workflow / a human curator.
