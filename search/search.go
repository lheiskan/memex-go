// Package search implements memex's keyword search and per-card filtering.
// Semantic / embedding search is intentionally omitted — see the porting
// notes in the project README.
package search

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"memex/card"
)

const DefaultLimit = 10

// Filter pre-filters cards by frontmatter fields before search runs.
// Empty fields are ignored. Match semantics mirror lib/commands/search.js.
type Filter struct {
	Category string
	Tag      string
	Author   string // matches frontmatter `author` OR `source`
	Since    string // YYYY-MM-DD; card created OR modified >= this
	Before   string // YYYY-MM-DD; card created OR modified <  this
}

func (f Filter) Empty() bool {
	return f == Filter{}
}

// Options collects flags for Run. Limit <= 0 falls back to DefaultLimit.
type Options struct {
	Limit  int
	Filter Filter
}

// Result is a single ranked search hit ready for output.
type Result struct {
	Slug           string
	Title          string
	FirstParagraph string
	MatchLine      string // empty if not relevant
	Links          []string
}

// Run executes a keyword search against the store. If query is empty,
// returns every card (after Filter) as a "list all" result with Title only.
// Token logic is AND: every whitespace-separated query token must appear
// (case-insensitive) in the card's title or body.
func Run(s *card.Store, query string, opts Options) ([]Result, bool, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	cards, err := s.ScanAll()
	if err != nil {
		return nil, false, err
	}
	if len(cards) == 0 {
		return nil, false, nil
	}

	if !opts.Filter.Empty() {
		cards, err = filterByFrontmatter(s, cards, opts.Filter)
		if err != nil {
			return nil, false, err
		}
		if len(cards) == 0 {
			return nil, false, nil
		}
	}

	if query == "" {
		// "list all" mode: just return slug + title for each card, no body.
		results := make([]Result, 0, len(cards))
		for _, c := range cards {
			raw, err := s.ReadCard(c.Slug)
			if err != nil {
				return nil, false, err
			}
			fm, _ := card.ParseFrontmatter(raw)
			results = append(results, Result{Slug: c.Slug, Title: titleOr(fm, c.Slug)})
		}
		return results, true, nil
	}

	tokens := strings.Fields(query)
	if len(tokens) == 0 {
		return nil, false, nil
	}
	patterns := make([]*regexp.Regexp, len(tokens))
	for i, t := range tokens {
		// Case-insensitive literal match (escaped, like JS replaces regex meta).
		patterns[i] = regexp.MustCompile(`(?i)` + regexp.QuoteMeta(t))
	}

	type scored struct {
		Result
		score int
	}
	var hits []scored
	for _, c := range cards {
		raw, err := s.ReadCard(c.Slug)
		if err != nil {
			return nil, false, err
		}
		fm, body := card.ParseFrontmatter(raw)
		title := titleOr(fm, c.Slug)
		searchText := title + "\n" + string(body)
		matched := true
		score := 0
		for _, re := range patterns {
			n := len(re.FindAllStringIndex(searchText, -1))
			if n == 0 {
				matched = false
				break
			}
			score += n
		}
		if !matched {
			continue
		}
		// First-token first-line preview, like JS reference.
		firstRe := patterns[0]
		var matchLine string
		for _, line := range strings.Split(string(body), "\n") {
			if firstRe.MatchString(line) {
				matchLine = strings.TrimSpace(line)
				break
			}
		}
		paras := strings.Split(strings.TrimSpace(string(body)), "\n\n")
		first := ""
		if len(paras) > 0 {
			first = strings.TrimSpace(paras[0])
		}
		// Don't double-print the match line if it's already in the first paragraph.
		if matchLine != "" && strings.Contains(first, matchLine) {
			matchLine = ""
		}
		hits = append(hits, scored{
			Result: Result{
				Slug:           c.Slug,
				Title:          title,
				FirstParagraph: first,
				MatchLine:      matchLine,
				Links:          card.ExtractLinks(body),
			},
			score: score,
		})
	}

	if len(hits) == 0 {
		return nil, false, nil
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	if len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]Result, len(hits))
	for i, h := range hits {
		out[i] = h.Result
	}
	return out, false, nil
}

func titleOr(fm card.Frontmatter, fallback string) string {
	if t := fm.GetString("title"); t != "" {
		return t
	}
	return fallback
}

func filterByFrontmatter(s *card.Store, cards []card.ScanEntry, f Filter) ([]card.ScanEntry, error) {
	var out []card.ScanEntry
	for _, c := range cards {
		raw, err := s.ReadCard(c.Slug)
		if err != nil {
			return nil, err
		}
		fm, _ := card.ParseFrontmatter(raw)
		if matchesFilter(fm, f) {
			out = append(out, c)
		}
	}
	return out, nil
}

func matchesFilter(fm card.Frontmatter, f Filter) bool {
	if f.Category != "" {
		got := fm.GetString("category")
		if !strings.EqualFold(got, f.Category) {
			return false
		}
	}
	if f.Tag != "" {
		needle := strings.ToLower(f.Tag)
		raw, ok := fm.Get("tags")
		if !ok {
			raw, ok = fm.Get("tag")
		}
		if !ok {
			return false
		}
		var tags []string
		switch v := raw.(type) {
		case []any:
			for _, e := range v {
				tags = append(tags, strings.ToLower(strings.TrimSpace(fmt.Sprint(e))))
			}
		case string:
			for _, t := range strings.Split(v, ",") {
				tags = append(tags, strings.ToLower(strings.TrimSpace(t)))
			}
		default:
			tags = append(tags, strings.ToLower(strings.TrimSpace(fmt.Sprint(v))))
		}
		found := false
		for _, t := range tags {
			if t == needle {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if f.Author != "" {
		needle := strings.ToLower(f.Author)
		if !strings.EqualFold(fm.GetString("author"), needle) &&
			!strings.EqualFold(fm.GetString("source"), needle) {
			return false
		}
	}
	if f.Since != "" {
		c, m := dateString(fm, "created"), dateString(fm, "modified")
		if !(c >= f.Since || m >= f.Since) {
			return false
		}
	}
	if f.Before != "" {
		c, m := dateString(fm, "created"), dateString(fm, "modified")
		cOk := c != "" && c < f.Before
		mOk := m != "" && m < f.Before
		if !cOk && !mOk {
			return false
		}
	}
	return true
}

// dateString returns the YYYY-MM-DD prefix of a frontmatter date field. Mirrors
// search.js's toDateString. Note: this slices the FIRST 10 chars, which is
// exactly the legacy bug for cards with `created: 'Thu Apr 16 ...'`. We keep
// the bug for behavior parity; document/fix later.
func dateString(fm card.Frontmatter, key string) string {
	s := fm.GetString(key)
	if len(s) > 10 {
		return s[:10]
	}
	return s
}
