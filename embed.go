// Package memex exists only to embed the repository's agent-facing assets.
//
// `go:embed` cannot reach outside its own package directory, so embedding
// skills/ from internal/claudecode would mean either a `../` (illegal) or
// moving the directory. A root package holding nothing but the directive is
// the cheapest way to keep skills/<name>/SKILL.md as the single source of
// truth.
//
// They are embedded rather than read from disk because `memex harness install`
// has to work with no repository checkout present. The directive reads at
// build time, so a freshly built binary cannot carry a stale copy.
package memex

import "embed"

// Skills holds the bundled SKILL.md files, keyed skills/<name>/SKILL.md.
//
//go:embed skills/*/SKILL.md
var Skills embed.FS
