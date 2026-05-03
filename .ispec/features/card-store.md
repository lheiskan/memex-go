---
type: feature
stable_id: F-1
slug: card-store
status: implemented
targets: [REQ-1]
paths:
  - card/parser.go
  - card/store.go
---

# Card store: parser, ordered frontmatter, atomic writes, slug validation

The kernel of the port. Anything that touches a card on disk goes
through this package.

## Surface

- `ParseFrontmatter(raw []byte) (Frontmatter, []byte)` — splits a card
  into ordered frontmatter + body. Falls back to empty metadata + raw
  body on YAML parse failure (matches the `gray-matter` permissive
  behavior in the JS reference).
- `StringifyFrontmatter(body []byte, fm Frontmatter) []byte` —
  hand-rolled emitter; see [[ADR-0002]] for the design rationale.
- `ExtractLinks(body []byte) []string` — `[[wikilink]]` extractor,
  unique-preserving order.
- `ValidateSlug(slug string) error` — mirrors the JS `validateSlug`
  rules (no reserved chars, no traversal, no empty path segments).
- `Store{CardsDir, ArchiveDir}` — `ScanAll`, `Resolve`, `ReadCard`,
  `WriteCard` (atomic via temp+rename), `ArchiveCard`. Scan results
  are memoized until the next write.

## Invariants

- `WriteCard` always validates the slug before touching disk and
  asserts the resolved path stays under `CardsDir`. No traversal
  out, even if a future caller passes attacker-controlled input.
- `Frontmatter.Fields` is an ordered slice, not a map. `Set` updates
  in place if the key exists, appends otherwise — preserving the
  original file's field order on round-trip.

## Verification

`card/parser_test.go::TestRoundTripRealCards` parses every `.md` file
under `$MEMEX_HOME/cards/` (default `~/.memex/cards/`), re-emits, and
counts byte diffs. Initial run: 200 cards, 0 diffs. This is the
load-bearing first test of the port — see [[ADR-0002]].
