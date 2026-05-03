package search

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"memex/card"
)

func newTestStore(t *testing.T, fixtures map[string]string) *card.Store {
	t.Helper()
	dir := t.TempDir()
	cardsDir := filepath.Join(dir, "cards")
	archiveDir := filepath.Join(dir, "archive")
	_ = os.MkdirAll(cardsDir, 0o755)
	_ = os.MkdirAll(archiveDir, 0o755)
	for slug, content := range fixtures {
		_ = os.WriteFile(filepath.Join(cardsDir, slug+".md"), []byte(content), 0o644)
	}
	return card.NewStore(cardsDir, archiveDir, false)
}

func TestKeywordSearch(t *testing.T) {
	s := newTestStore(t, map[string]string{
		"alpha": "---\ntitle: Alpha\n---\nthe quick brown fox\n",
		"beta":  "---\ntitle: Beta\n---\nfox jumps over fox lazy dog\n",
		"gamma": "---\ntitle: Gamma\n---\nunrelated content here\n",
	})
	out, _, err := Run(s, "fox", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(out))
	}
	// Beta has 2 occurrences of "fox", should rank first.
	if out[0].Slug != "beta" {
		t.Errorf("expected beta first, got %s", out[0].Slug)
	}
}

func TestSearchListAll(t *testing.T) {
	s := newTestStore(t, map[string]string{
		"a": "---\ntitle: A\n---\nbody\n",
		"b": "---\ntitle: B\n---\nbody\n",
	})
	out, listAll, err := Run(s, "", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !listAll {
		t.Errorf("expected listAll mode")
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 results, got %d", len(out))
	}
}

func TestSearchAndLogic(t *testing.T) {
	s := newTestStore(t, map[string]string{
		"both": "---\ntitle: T\n---\napple banana\n",
		"one":  "---\ntitle: T\n---\napple only\n",
	})
	out, _, _ := Run(s, "apple banana", Options{})
	if len(out) != 1 || out[0].Slug != "both" {
		t.Errorf("AND logic broken: got %v", out)
	}
}

func TestFilterByTag(t *testing.T) {
	s := newTestStore(t, map[string]string{
		"a": "---\ntitle: A\ntags: [foo, bar]\n---\nbody\n",
		"b": "---\ntitle: B\ntags: 'foo,baz'\n---\nbody\n",
		"c": "---\ntitle: C\ntags: [other]\n---\nbody\n",
	})
	out, _, _ := Run(s, "", Options{Filter: Filter{Tag: "foo"}})
	if len(out) != 2 {
		t.Errorf("expected 2 cards with tag foo, got %d (%v)", len(out), slugsOf(out))
	}
}

func slugsOf(rs []Result) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Slug
	}
	return out
}

func TestFilterSinceBefore(t *testing.T) {
	s := newTestStore(t, map[string]string{
		"old": "---\ntitle: O\ncreated: 2025-01-01\n---\n",
		"mid": "---\ntitle: M\ncreated: 2026-01-01\n---\n",
		"new": "---\ntitle: N\ncreated: 2026-06-01\n---\n",
	})
	out, _, _ := Run(s, "", Options{Filter: Filter{Since: "2026-01-01", Before: "2026-06-01"}})
	if len(out) != 1 || out[0].Slug != "mid" {
		t.Errorf("date filter wrong: got %v", slugsOf(out))
	}
	_ = strings.Join // keep import
}
