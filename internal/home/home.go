// Package home resolves where the card store lives.
//
// One answer to "where is the store", so the CLI, the MCP server and the
// harness hook cannot disagree about it.
package home

import (
	"os"
	"path/filepath"

	"memex/card"
)

// Dir returns $MEMEX_HOME, or ~/.memex.
//
// Returns an error rather than exiting: one caller is a session-start hook
// that must never break a session, and it needs to decide that for itself.
func Dir() (string, error) {
	if h := os.Getenv("MEMEX_HOME"); h != "" {
		return h, nil
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, ".memex"), nil
}

// Store opens the card store under Dir.
//
// NestedSlugs is false, matching the CLI: cards/a/b.md resolves as slug "b".
func Store(dir string) *card.Store {
	return card.NewStore(filepath.Join(dir, "cards"), filepath.Join(dir, "archive"), false)
}
