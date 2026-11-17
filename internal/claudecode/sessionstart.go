package claudecode

import (
	"fmt"
	"strings"

	"memex/status"
)

// SessionStartText is what the SessionStart hook prints. It primes the agent
// with knowledge of the memex memory system and how to use it.
func SessionStartText(f status.Facts) string {
	var b strings.Builder

	if f.Cards == 0 {
		b.WriteString("## Memex Memory System Active (empty)\n\n")
		b.WriteString("There are no cards yet. Searching will find nothing, which is worth\n")
		b.WriteString("knowing before you conclude the memory has no answer. When this session\n")
		b.WriteString("turns up something reusable, propose a card at the end and let the user\n")
		b.WriteString("decide whether to keep it.\n")
		return b.String()
	}

	fmt.Fprintf(&b, "## Memex Memory System Active (%d %s", f.Cards, plural(f.Cards, "card"))
	if n := len(f.Sections); n > 0 {
		fmt.Fprintf(&b, ", %d %s", n, plural(n, "topic"))
	}
	b.WriteString(")\n")

	// The knowledge map. Upstream builds this with an awk program inlined in
	// its hook command; package status does it in Go, so this is a loop.
	if len(f.Sections) > 0 {
		b.WriteString("\n### What is in there\n\n")
		for _, s := range f.Sections {
			fmt.Fprintf(&b, "- %s (%d %s)\n", s.Title, s.Cards, plural(s.Cards, "card"))
		}
	}

	b.WriteString(`
### Recall — before you start

When the task touches any topic above, read before you write. Either invoke
the ` + "`memex-recall`" + ` skill, or go straight to the store:

    memex search --limit 5 <keyword>    # flags BEFORE the query
    memex read <slug>                   # a slug list is not the memory

Search is a case-insensitive AND over tokens, so use 1-3 keywords. A sentence
or a task summary matches nothing and returns an empty result that reads
exactly like "the memory has nothing on this".

The topic list comes from the keyword index, which only knows cards that
existed when ` + "`memex organize`" + ` last ran. If this task continues recent work,
search as well as reading the index.

### Retro — at the end

After work that involved code changes, debugging, or a non-trivial decision,
invoke the ` + "`memex-retro`" + ` skill rather than waiting to be asked. Draft the
cards, then name them and what each claims — the user decides what gets
stored.

Never put secrets, credentials, tokens or exact secret file contents in a
card. Describe them abstractly or redact them.
`)

	// Staleness is load-bearing, not a maintenance nag: a stale index means
	// recall's primary entry point is degraded and search has to carry the
	// task. -1 is how status reports "never ran", including an unparseable
	// date — upstream renders that case as "0 days ago", the most misleading
	// answer available.
	switch {
	case f.DaysSinceOrganize < 0:
		b.WriteString("\nNote: `memex organize` has never run, so there is no keyword index.\n")
	case f.DaysSinceOrganize >= staleAfterDays:
		fmt.Fprintf(&b, "\nNote: last organize was %d days ago. The topic list above is missing\n", f.DaysSinceOrganize)
		b.WriteString("everything written since, so prefer search over the index this session.\n")
	}
	return b.String()
}

// staleAfterDays is when the index stops being a current map and starts being
// a historical document. Seven days matches what package session used.
const staleAfterDays = 7

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
