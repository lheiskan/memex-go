// Package graph implements link-graph analysis: outbound/inbound link stats,
// backlinks, and the organize report (orphans, hubs, conflicts, recently
// modified pairs for contradiction detection).
package graph

import (
	"fmt"
	"sort"
	"strings"

	"memex/card"
)

const HubThreshold = 10

type Stat struct {
	Slug     string
	Outbound int
	Inbound  int
}

// Build walks every card once and returns the link maps + per-card metadata.
type Maps struct {
	Outbound map[string][]string
	Inbound  map[string][]string
	Cards    []card.ScanEntry
	Meta     map[string]CardMeta
}

type CardMeta struct {
	Title    string
	Modified string
	Status   string
	Body     string
}

func Build(s *card.Store) (Maps, error) {
	cards, err := s.ScanAll()
	if err != nil {
		return Maps{}, err
	}
	m := Maps{
		Outbound: make(map[string][]string, len(cards)),
		Inbound:  make(map[string][]string, len(cards)),
		Meta:     make(map[string]CardMeta, len(cards)),
		Cards:    cards,
	}
	for _, c := range cards {
		m.Inbound[c.Slug] = nil
	}
	for _, c := range cards {
		raw, err := s.ReadCard(c.Slug)
		if err != nil {
			return Maps{}, err
		}
		fm, body := card.ParseFrontmatter(raw)
		links := card.ExtractLinks(body)
		m.Outbound[c.Slug] = links
		m.Meta[c.Slug] = CardMeta{
			Title:    titleOr(fm, c.Slug),
			Modified: dateString(fm.GetString("modified")),
			Status:   fm.GetString("status"),
			Body:     strings.TrimSpace(string(body)),
		}
		for _, l := range links {
			m.Inbound[l] = append(m.Inbound[l], c.Slug)
		}
	}
	return m, nil
}

// Stats returns one Stat per card, in scan order.
func (m Maps) Stats() []Stat {
	out := make([]Stat, len(m.Cards))
	for i, c := range m.Cards {
		out[i] = Stat{
			Slug:     c.Slug,
			Outbound: len(m.Outbound[c.Slug]),
			Inbound:  len(m.Inbound[c.Slug]),
		}
	}
	return out
}

// Backlinks returns the slugs that link TO `target` via [[wikilinks]].
func Backlinks(s *card.Store, target string) ([]string, error) {
	m, err := Build(s)
	if err != nil {
		return nil, err
	}
	out := append([]string(nil), m.Inbound[target]...)
	sort.Strings(out)
	return out, nil
}

// Organize produces the network maintenance report. lastOrganize is a
// YYYY-MM-DD string; if non-empty, the recently-modified pair detection
// limits to cards whose `modified` >= that date. Empty = first run, all cards.
func Organize(s *card.Store, lastOrganize string) (string, error) {
	m, err := Build(s)
	if err != nil {
		return "", err
	}
	if len(m.Cards) == 0 {
		return "No cards yet.", nil
	}
	stats := m.Stats()

	var sections []string
	sections = append(sections, "# Organize Report\n")
	sections = append(sections, "## Link Stats\n"+formatStats(stats))

	var orphans []Stat
	for _, st := range stats {
		if st.Inbound == 0 && st.Slug != "index" {
			orphans = append(orphans, st)
		}
	}
	if len(orphans) > 0 {
		var b strings.Builder
		b.WriteString("## Orphans (no inbound links)\n")
		for _, o := range orphans {
			fmt.Fprintf(&b, "- %s — %s\n", o.Slug, m.Meta[o.Slug].Title)
		}
		sections = append(sections, strings.TrimRight(b.String(), "\n"))
	}

	var hubs []Stat
	for _, st := range stats {
		if st.Inbound >= HubThreshold {
			hubs = append(hubs, st)
		}
	}
	if len(hubs) > 0 {
		var b strings.Builder
		b.WriteString("## Hubs (≥10 inbound links)\n")
		for _, h := range hubs {
			fmt.Fprintf(&b, "- %s (%d inbound) — %s\n", h.Slug, h.Inbound, m.Meta[h.Slug].Title)
		}
		sections = append(sections, strings.TrimRight(b.String(), "\n"))
	}

	var conflicts []string
	for _, c := range m.Cards {
		if m.Meta[c.Slug].Status == "conflict" {
			conflicts = append(conflicts, c.Slug)
		}
	}
	if len(conflicts) > 0 {
		var b strings.Builder
		b.WriteString("## Unresolved Conflicts\n")
		for _, slug := range conflicts {
			fmt.Fprintf(&b, "- %s — %s\n", slug, m.Meta[slug].Title)
		}
		sections = append(sections, strings.TrimRight(b.String(), "\n"))
	}

	// Recently modified + neighbors for contradiction review.
	var recent []string
	for _, c := range m.Cards {
		mod := m.Meta[c.Slug].Modified
		if lastOrganize == "" || mod == "" || mod >= lastOrganize {
			recent = append(recent, c.Slug)
		}
	}
	if len(recent) > 0 {
		var pairs []string
		seen := map[string]bool{}
		for _, slug := range recent {
			info := m.Meta[slug]
			for _, neighbor := range m.Outbound[slug] {
				ninfo, ok := m.Meta[neighbor]
				if !ok {
					continue
				}
				key := pairKey(slug, neighbor)
				if seen[key] {
					continue
				}
				seen[key] = true
				pairs = append(pairs, fmt.Sprintf(
					"### %s ↔ %s\n**%s** (%s):\n%s\n\n**%s** (%s):\n%s",
					slug, neighbor, slug, info.Title, excerpt(info.Body, 300),
					neighbor, ninfo.Title, excerpt(ninfo.Body, 300),
				))
			}
		}
		if len(pairs) > 0 {
			capped := pairs
			suffix := ""
			if len(pairs) > 20 {
				capped = pairs[:20]
				suffix = fmt.Sprintf("\n\n... and %d more pairs not shown. Run with a recent `since` date for targeted checks.", len(pairs)-20)
			}
			sections = append(sections,
				"## Recently Modified Cards + Neighbors (check for contradictions)\n"+
					strings.Join(capped, "\n\n")+suffix)
		}
	}

	return strings.Join(sections, "\n\n"), nil
}

func pairKey(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + "↔" + b
}

func excerpt(body string, n int) string {
	if len(body) <= n {
		return body
	}
	return body[:n]
}

func titleOr(fm card.Frontmatter, fallback string) string {
	if t := fm.GetString("title"); t != "" {
		return t
	}
	return fallback
}

func dateString(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

// FormatStats is exported so the CLI links command can reuse the table layout.
func FormatStats(stats []Stat) string { return formatStats(stats) }

func formatStats(stats []Stat) string {
	if len(stats) == 0 {
		return ""
	}
	maxLen := 0
	for _, s := range stats {
		if len(s.Slug) > maxLen {
			maxLen = len(s.Slug)
		}
	}
	pad := func(s string, n int) string {
		if len(s) >= n {
			return s
		}
		return s + strings.Repeat(" ", n-len(s))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s%s%s%s\n", pad("slug", maxLen+2), pad("out", 5), pad("in", 5), "status")
	for _, s := range stats {
		status := ""
		if s.Inbound == 0 {
			status = "orphan"
		} else if s.Inbound >= HubThreshold {
			status = "hub"
		}
		fmt.Fprintf(&b, "%s%s%s%s\n",
			pad(s.Slug, maxLen+2),
			pad(fmt.Sprintf("%d", s.Outbound), 5),
			pad(fmt.Sprintf("%d", s.Inbound), 5),
			status)
	}
	return strings.TrimRight(b.String(), "\n")
}

// FormatCardLinks formats outbound + inbound for a single slug.
func FormatCardLinks(slug string, outbound, inbound []string) string {
	wikify := func(slugs []string) string {
		if len(slugs) == 0 {
			return "(none)"
		}
		parts := make([]string, len(slugs))
		for i, s := range slugs {
			parts[i] = "[[" + s + "]]"
		}
		return strings.Join(parts, ", ")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\nOutbound: %s\nInbound:  %s",
		slug, wikify(outbound), wikify(inbound))
	return b.String()
}
