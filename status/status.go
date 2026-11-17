// Package status reports facts about the card store.
package status

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"memex/card"
	"memex/sync"
)

// Section is one `## ` heading of the keyword index and the number of cards
// listed under it.
type Section struct {
	Title string
	Cards int
}

// Facts is everything the store can be asked without judgement.
type Facts struct {
	Cards             int       // cards in the store, excluding the archive
	IndexPresent      bool      // whether an `index` card exists
	IndexEntries      int       // `- [[slug]]` lines in the index
	Sections          []Section // index sections, in document order
	LastOrganize      string    // YYYY-MM-DD, or "" if organize never ran
	DaysSinceOrganize int       // -1 when LastOrganize is ""
	SyncConfigured    bool
}

// Collect gathers the facts. Errors are not fatal: a store that cannot be
// scanned reports zero cards rather than failing, because the only caller is a
// session-start hook that must not break a session.
func Collect(s *card.Store, home string) Facts {
	return collectAt(s, home, time.Now().UTC())
}

func collectAt(s *card.Store, home string, now time.Time) Facts {
	f := Facts{DaysSinceOrganize: -1}

	if entries, err := s.ScanAll(); err == nil {
		f.Cards = len(entries)
	}

	if raw, err := s.ReadCard("index"); err == nil {
		f.IndexPresent = true
		_, body := card.ParseFrontmatter(raw)
		f.Sections, f.IndexEntries = parseIndex(body)
	}

	if b, err := os.ReadFile(filepath.Join(home, ".last-organize")); err == nil {
		d := strings.TrimSpace(string(b))
		if t, perr := time.Parse("2006-01-02", d); perr == nil {
			f.LastOrganize = d
			f.DaysSinceOrganize = int(now.Sub(t).Hours() / 24)
		}
	}

	f.SyncConfigured = sync.ReadConfig(home).Remote != ""
	return f
}

// parseIndex counts `- [[slug]]` entries under each `## ` heading.
//
// This is the awk program upstream inlines in its hook command, moved into Go
// where it can be tested. Entries before the first heading are counted in the
// total but belong to no section.
func parseIndex(body []byte) ([]Section, int) {
	var sections []Section
	total := 0
	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "## "):
			sections = append(sections, Section{Title: strings.TrimSpace(line[3:])})
		case strings.HasPrefix(line, "- [["):
			total++
			if n := len(sections); n > 0 {
				sections[n-1].Cards++
			}
		}
	}
	return sections, total
}

// Format renders the facts as scalar `key: value` lines, then a blank line,
// then one `- <title> (<n> cards)` line per index section.
//
// Deliberately boring, so a consumer can read the scalars with grep and echo
// the section lines verbatim — no awk, no python3, and in particular no date
// arithmetic of its own. Staleness is reported as an integer here precisely so
// nothing downstream has to compute it.
func Format(f Facts) string {
	var b strings.Builder
	fmt.Fprintf(&b, "cards: %d\n", f.Cards)
	fmt.Fprintf(&b, "index: %s\n", presence(f.IndexPresent))
	fmt.Fprintf(&b, "index_entries: %d\n", f.IndexEntries)
	fmt.Fprintf(&b, "sections: %d\n", len(f.Sections))
	fmt.Fprintf(&b, "last_organize: %s\n", orNever(f.LastOrganize))
	fmt.Fprintf(&b, "days_since_organize: %d\n", f.DaysSinceOrganize)
	fmt.Fprintf(&b, "sync: %s\n", configured(f.SyncConfigured))
	if len(f.Sections) > 0 {
		b.WriteString("\n")
		for _, s := range f.Sections {
			fmt.Fprintf(&b, "- %s (%d %s)\n", s.Title, s.Cards, plural(s.Cards, "card"))
		}
	}
	return b.String()
}

func presence(b bool) string {
	if b {
		return "present"
	}
	return "absent"
}

func configured(b bool) string {
	if b {
		return "configured"
	}
	return "unconfigured"
}

func orNever(s string) string {
	if s == "" {
		return "never"
	}
	return s
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
