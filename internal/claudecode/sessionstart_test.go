package claudecode

import (
	"strings"
	"testing"

	"memex/status"
)

// These branches were previously verified by running a shell script by hand
// against scratch stores. In Go the wording is assertable, which is what
// keeps it from drifting.

func TestSessionStartEmptyStore(t *testing.T) {
	out := SessionStartText(status.Facts{DaysSinceOrganize: -1})

	if !strings.Contains(out, "(empty)") {
		t.Errorf("want the empty-store variant:\n%s", out)
	}
	// An agent that knows the memory is empty will not read a failed search
	// as "no answer exists".
	if !strings.Contains(out, "Searching will find nothing") {
		t.Errorf("want the no-answer warning:\n%s", out)
	}
	// None of the recall machinery is worth printing against zero cards.
	for _, unwanted := range []string{"memex search", "### What is in there", "organize"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("empty-store text should not mention %q:\n%s", unwanted, out)
		}
	}
}

func TestSessionStartKnowledgeMap(t *testing.T) {
	out := SessionStartText(status.Facts{
		Cards:             42,
		DaysSinceOrganize: 2,
		Sections: []status.Section{
			{Title: "Go Patterns", Cards: 24},
			{Title: "Lone Topic", Cards: 1},
		},
	})

	for _, want := range []string{
		"## Memex Memory System Active (42 cards, 2 topics)",
		"### What is in there",
		"- Go Patterns (24 cards)",
		"- Lone Topic (1 card)", // singular
		"### Recall — before you start",
		"### Retro — at the end",
		"flags BEFORE the query",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	// Fresh organize: no staleness note at all.
	if strings.Contains(out, "Note:") {
		t.Errorf("unexpected note for a 2-day-old index:\n%s", out)
	}
}

func TestSessionStartStaleIndex(t *testing.T) {
	out := SessionStartText(status.Facts{Cards: 470, DaysSinceOrganize: 180})

	if !strings.Contains(out, "last organize was 180 days ago") {
		t.Errorf("want the staleness note:\n%s", out)
	}
	// Framed as degraded recall, not as overdue maintenance.
	if !strings.Contains(out, "prefer search over the index") {
		t.Errorf("want the note to say what to do instead:\n%s", out)
	}
}

func TestSessionStartOrganizeNeverRan(t *testing.T) {
	out := SessionStartText(status.Facts{Cards: 3, DaysSinceOrganize: -1})

	if !strings.Contains(out, "has never run") {
		t.Errorf("want the never-ran note:\n%s", out)
	}
	// -1 must never render as a day count. Upstream's python3 fallback turns
	// an unparseable date into "0 days ago", the most misleading answer going.
	if strings.Contains(out, "-1 days") || strings.Contains(out, "0 days ago") {
		t.Errorf("never-ran rendered as a day count:\n%s", out)
	}
}

func TestSessionStartStalenessBoundary(t *testing.T) {
	if out := SessionStartText(status.Facts{Cards: 1, DaysSinceOrganize: staleAfterDays - 1}); strings.Contains(out, "Note:") {
		t.Errorf("day %d should be quiet:\n%s", staleAfterDays-1, out)
	}
	if out := SessionStartText(status.Facts{Cards: 1, DaysSinceOrganize: staleAfterDays}); !strings.Contains(out, "Note:") {
		t.Errorf("day %d should warn:\n%s", staleAfterDays, out)
	}
}

// The register matters: this is a reversal of upstream's "MANDATORY / you
// MUST / NOW", and the retro guidance has to leave the decision with the user.
func TestSessionStartProposesRatherThanCommands(t *testing.T) {
	out := SessionStartText(status.Facts{Cards: 10, DaysSinceOrganize: 1})

	for _, shouting := range []string{"MANDATORY", "you MUST", "NOW before ending"} {
		if strings.Contains(out, shouting) {
			t.Errorf("found %q in the directive:\n%s", shouting, out)
		}
	}
	if !strings.Contains(out, "the user decides what") {
		t.Errorf("retro guidance must leave the decision with the user:\n%s", out)
	}
	if !strings.Contains(out, "Never put secrets") {
		t.Errorf("missing the secrets warning:\n%s", out)
	}
}
