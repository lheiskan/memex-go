---
type: adr
stable_id: ADR-0002
slug: byte-identical-yaml-roundtrip-via-node-api
status: accepted
relates_to: [REQ-1]
---

# Hand-rolled YAML emitter against yaml.v3 Node-API decoder for byte-identical round-trip

## Context

Card frontmatter is YAML inside `---` fences. The upstream JS reference
(`lib/parser.js`) uses `gray-matter` (which wraps `js-yaml`) to parse,
but does NOT use `js-yaml` to emit — it has a hand-rolled stringifier
with a specific quoting rule (single-quoted strings, doubled `'`,
empty-or-special-char trigger), insertion-order field iteration, and
some lossy normalizations (newlines flattened to spaces, arrays
joined with `,`).

Naively using `gopkg.in/yaml.v3`'s `Encoder.Encode` in the Go port
would have caused gratuitous diffs on every card re-save:

- Map iteration order in Go is randomised, so field ordering churns.
- `time.Time` would emit as `2026-05-03T00:00:00Z` instead of `2026-05-03`.
- The encoder may pick double-quoted strings, block scalars (`|`, `>-`),
  or different quoting rules than the JS reference.

That would invalidate [[REQ-1]] (drop-in compatibility) the moment a
card was re-written through the Go server, polluting the user's
`~/.memex/.git` history with thousands of cosmetic changes.

## Decision

Use `gopkg.in/yaml.v3`'s **Node API** for parsing only — decode the
top-level mapping into an ordered `[]Field{Key, Value}` by walking the
`MappingNode.Content` slice pairwise. Hand-roll the emitter to mirror
the JS reference's quoting rules exactly, including the lossy
normalisations.

Concretely (`card/parser.go`):

- `ParseFrontmatter` decodes via `yaml.Node`, preserving field order.
- `StringifyFrontmatter` iterates ordered fields, applies the same
  special-char regex (`[:#{}[\],&*?|>!%@`']`) for quote-trigger, and
  doubles single quotes inside quoted values.
- `time.Time` is normalised to `2006-01-02` (matching the JS write
  path's `created`-coercion behavior).
- `[]any` is joined with `,` (matching JS `String(arr)`); the array
  shape is lost, just like in upstream.

## Consequences

- Round-trip on all 200 of the user's real cards: zero byte diffs on
  the first run. (See [[F-1]].) This was the load-bearing first test
  of the port.
- Future schema additions (e.g. emitting array-typed `tags` properly)
  must trade off "fix the lossy upstream behaviour" against "stop
  matching upstream's writer." Document any such deviation as its
  own ADR.
- `yaml.v3`'s native `Marshal` is intentionally not used anywhere in
  the writer path. If a future contributor adds a `yaml.Marshal` call,
  they will silently break round-trip — guard with a lint or a
  parser_test fixture.
