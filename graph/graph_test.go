package graph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"memex/card"
)

func newStore(t *testing.T, fixtures map[string]string) *card.Store {
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

func TestBuildAndStats(t *testing.T) {
	s := newStore(t, map[string]string{
		"a": "---\ntitle: A\n---\nbody links to [[b]] and [[c]]\n",
		"b": "---\ntitle: B\n---\nbody links to [[c]]\n",
		"c": "---\ntitle: C\n---\nno links\n",
	})
	m, err := Build(s)
	if err != nil {
		t.Fatal(err)
	}
	stats := map[string]Stat{}
	for _, st := range m.Stats() {
		stats[st.Slug] = st
	}
	if stats["a"].Outbound != 2 {
		t.Errorf("a outbound: got %d, want 2", stats["a"].Outbound)
	}
	if stats["c"].Inbound != 2 {
		t.Errorf("c inbound: got %d, want 2", stats["c"].Inbound)
	}
}

func TestBacklinks(t *testing.T) {
	s := newStore(t, map[string]string{
		"target": "---\ntitle: T\n---\n",
		"a":      "---\ntitle: A\n---\nrefers to [[target]]\n",
		"b":      "---\ntitle: B\n---\nalso [[target]]\n",
		"c":      "---\ntitle: C\n---\nirrelevant\n",
	})
	bl, err := Backlinks(s, "target")
	if err != nil {
		t.Fatal(err)
	}
	if len(bl) != 2 || bl[0] != "a" || bl[1] != "b" {
		t.Errorf("backlinks: got %v, want [a b]", bl)
	}
}

func TestOrganizeBasic(t *testing.T) {
	s := newStore(t, map[string]string{
		"index":  "---\ntitle: Index\n---\nlinks to [[a]] [[b]]\n",
		"a":      "---\ntitle: A\nstatus: conflict\n---\n[[b]]\n",
		"b":      "---\ntitle: B\n---\nbody\n",
		"orphan": "---\ntitle: O\n---\nnobody links here\n",
	})
	out, err := Organize(s, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Orphans") || !strings.Contains(out, "orphan") {
		t.Errorf("organize missing orphans section:\n%s", out)
	}
	if !strings.Contains(out, "Unresolved Conflicts") {
		t.Errorf("organize missing conflicts section")
	}
	if strings.Contains(out, "- index") {
		t.Errorf("index should be excluded from orphans")
	}
}
