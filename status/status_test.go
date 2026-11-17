package status

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"memex/card"
)

func newTestStore(t *testing.T) (*card.Store, string) {
	t.Helper()
	home := t.TempDir()
	cards := filepath.Join(home, "cards")
	if err := os.MkdirAll(cards, 0o755); err != nil {
		t.Fatal(err)
	}
	return card.NewStore(cards, filepath.Join(home, "archive"), false), home
}

func writeCard(t *testing.T, s *card.Store, slug, body string) {
	t.Helper()
	content := "---\ntitle: " + slug + "\ncreated: 2026-11-17\nsource: test\n---\n\n" + body + "\n"
	if err := s.WriteCard(slug, []byte(content)); err != nil {
		t.Fatal(err)
	}
}

const indexBody = `Keyword Index

## Containers
- [[docker-thing]] — a thing
- [[podman-thing]] — another

## Go Gotchas
- [[go-flag-stops]] — the flag footgun
- [[go-json-escape]] — escaping
- [[go-servemux]] — routing
`

func TestParseIndexCountsPerSection(t *testing.T) {
	sections, total := parseIndex([]byte(indexBody))

	if total != 5 {
		t.Errorf("total entries = %d, want 5", total)
	}
	if len(sections) != 2 {
		t.Fatalf("sections = %d, want 2", len(sections))
	}
	if sections[0].Title != "Containers" || sections[0].Cards != 2 {
		t.Errorf("section 0 = %+v, want {Containers 2}", sections[0])
	}
	if sections[1].Title != "Go Gotchas" || sections[1].Cards != 3 {
		t.Errorf("section 1 = %+v, want {Go Gotchas 3}", sections[1])
	}
}

// Entries listed before any heading must still reach the total; a count that
// silently dropped them would make the knowledge map disagree with "N cards".
func TestParseIndexCountsEntriesBeforeFirstHeading(t *testing.T) {
	sections, total := parseIndex([]byte("- [[loose-one]] — x\n\n## S\n- [[in-section]] — y\n"))
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(sections) != 1 || sections[0].Cards != 1 {
		t.Errorf("sections = %+v, want one section holding 1", sections)
	}
}

func TestCollectWithNoIndexAndNoOrganize(t *testing.T) {
	s, home := newTestStore(t)
	writeCard(t, s, "alpha", "Body.")
	writeCard(t, s, "beta", "Body.")

	f := Collect(s, home)

	if f.Cards != 2 {
		t.Errorf("Cards = %d, want 2", f.Cards)
	}
	if f.IndexPresent {
		t.Error("IndexPresent = true, want false")
	}
	if f.DaysSinceOrganize != -1 {
		t.Errorf("DaysSinceOrganize = %d, want -1", f.DaysSinceOrganize)
	}
	if f.SyncConfigured {
		t.Error("SyncConfigured = true, want false")
	}

	out := Format(f)
	for _, want := range []string{"cards: 2", "index: absent", "last_organize: never", "days_since_organize: -1", "sync: unconfigured"} {
		if !strings.Contains(out, want) {
			t.Errorf("Format() missing %q:\n%s", want, out)
		}
	}
	// No sections means no list block at all, so the hook prints no empty map.
	if strings.Contains(out, "\n- ") {
		t.Errorf("Format() emitted a section list with no sections:\n%s", out)
	}
}

func TestCollectStalenessIsADayCount(t *testing.T) {
	s, home := newTestStore(t)
	if err := os.WriteFile(filepath.Join(home, ".last-organize"), []byte("2026-11-14\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

	f := collectAt(s, home, now)

	if f.LastOrganize != "2026-11-14" {
		t.Errorf("LastOrganize = %q, want 2026-11-14", f.LastOrganize)
	}
	if f.DaysSinceOrganize != 3 {
		t.Errorf("DaysSinceOrganize = %d, want 3", f.DaysSinceOrganize)
	}
}

// A malformed date must read as "never" rather than as a wrong number.
// Upstream's `|| echo $(date +%s)` fallback renders a parse failure as
// "0 days ago", which is the most misleading answer available.
func TestCollectMalformedOrganizeDateReadsAsNever(t *testing.T) {
	s, home := newTestStore(t)
	if err := os.WriteFile(filepath.Join(home, ".last-organize"), []byte("not-a-date\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	f := Collect(s, home)

	if f.LastOrganize != "" {
		t.Errorf("LastOrganize = %q, want empty", f.LastOrganize)
	}
	if f.DaysSinceOrganize != -1 {
		t.Errorf("DaysSinceOrganize = %d, want -1", f.DaysSinceOrganize)
	}
}

func TestFormatEmitsSectionList(t *testing.T) {
	s, home := newTestStore(t)
	writeCard(t, s, "index", indexBody)

	out := Format(Collect(s, home))

	for _, want := range []string{
		"index: present",
		"index_entries: 5",
		"sections: 2",
		"- Containers (2 cards)",
		"- Go Gotchas (3 cards)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Format() missing %q:\n%s", want, out)
		}
	}
}

func TestFormatPluralizesSectionCounts(t *testing.T) {
	out := Format(Facts{Sections: []Section{{Title: "One", Cards: 1}, {Title: "Two", Cards: 2}}})
	if !strings.Contains(out, "- One (1 card)") {
		t.Errorf("want singular form:\n%s", out)
	}
	if !strings.Contains(out, "- Two (2 cards)") {
		t.Errorf("want plural form:\n%s", out)
	}
}
