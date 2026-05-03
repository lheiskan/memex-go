package card

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseStringifyBasic(t *testing.T) {
	raw := []byte("---\ntitle: Hello\ncreated: 2026-05-03\nsource: claude-code\n---\nbody text\n")
	fm, body := ParseFrontmatter(raw)
	if fm.GetString("title") != "Hello" {
		t.Errorf("title: got %q, want Hello", fm.GetString("title"))
	}
	if fm.GetString("created") != "2026-05-03" {
		t.Errorf("created: got %q, want 2026-05-03", fm.GetString("created"))
	}
	if string(body) != "body text\n" {
		t.Errorf("body: got %q, want %q", string(body), "body text\n")
	}
	out := StringifyFrontmatter(body, fm)
	if !bytes.Equal(out, raw) {
		t.Errorf("round-trip mismatch:\nwant:\n%s\ngot:\n%s", raw, out)
	}
}

func TestStringifyQuotingRules(t *testing.T) {
	cases := []struct {
		key, value, want string
	}{
		{"a", "plain", "a: plain"},
		{"a", "has: colon", "a: 'has: colon'"},
		{"a", "has,comma", "a: 'has,comma'"},
		{"a", "can't", "a: 'can''t'"},
		{"a", "", "a: ''"},
		{"a", "#hash", "a: '#hash'"},
	}
	for _, tc := range cases {
		fm := Frontmatter{Fields: []Field{{Key: tc.key, Value: tc.value}}}
		out := StringifyFrontmatter(nil, fm)
		// out is "---\n<line>\n---\n"
		got := strings.TrimSuffix(strings.TrimPrefix(string(out), "---\n"), "\n---\n")
		if got != tc.want {
			t.Errorf("Stringify(%s=%q): got %q, want %q", tc.key, tc.value, got, tc.want)
		}
	}
}

func TestExtractLinksUnique(t *testing.T) {
	body := []byte("see [[foo]] and [[bar]] again [[foo]]")
	got := ExtractLinks(body)
	want := []string{"foo", "bar"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMalformedYAMLFallback(t *testing.T) {
	raw := []byte("---\ntitle: foo: bar\n---\nbody\n")
	fm, body := ParseFrontmatter(raw)
	if len(fm.Fields) != 0 {
		t.Errorf("expected empty fields on malformed YAML, got %v", fm.Fields)
	}
	if string(body) != "body\n" {
		t.Errorf("body: got %q, want body\\n", string(body))
	}
}

func TestNoFrontmatter(t *testing.T) {
	raw := []byte("just some text\n")
	fm, body := ParseFrontmatter(raw)
	if len(fm.Fields) != 0 {
		t.Errorf("expected empty fields, got %v", fm.Fields)
	}
	if !bytes.Equal(body, raw) {
		t.Errorf("body should be raw, got %q", string(body))
	}
}

// TestRoundTripRealCards parses every card in $MEMEX_HOME/cards/ (or
// ~/.memex/cards/ by default), re-stringifies, and reports any byte diffs.
//
// This is the load-bearing test for the port: it tells us the exact set of
// real-world cards that would churn on first save under the Go writer. It is
// read-only (never writes back to ~/.memex).
//
// The test is informational on the first run — it logs diffs but does not fail
// unless the diff rate exceeds an unreasonable threshold.
func TestRoundTripRealCards(t *testing.T) {
	home := os.Getenv("MEMEX_HOME")
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			t.Skip("no home dir available")
		}
		home = filepath.Join(h, ".memex")
	}
	cardsDir := filepath.Join(home, "cards")
	if _, err := os.Stat(cardsDir); os.IsNotExist(err) {
		t.Skipf("no cards dir at %s", cardsDir)
	}

	var total, diffs int
	var diffSlugs []string
	err := filepath.Walk(cardsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		total++
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fm, body := ParseFrontmatter(raw)
		out := StringifyFrontmatter(body, fm)
		if !bytes.Equal(raw, out) {
			diffs++
			rel, _ := filepath.Rel(cardsDir, path)
			diffSlugs = append(diffSlugs, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("round-tripped %d cards, %d diffs", total, diffs)
	if total == 0 {
		t.Skip("no cards found")
	}
	// Print up to 10 diffing slugs so we can investigate.
	for i, s := range diffSlugs {
		if i >= 10 {
			t.Logf("  ... and %d more", len(diffSlugs)-10)
			break
		}
		t.Logf("  diff: %s", s)
	}
	// Fail only if more than half churn — that signals a systematic emitter bug.
	if diffs*2 > total {
		t.Errorf("too many cards diff (%d/%d) — emitter probably wrong", diffs, total)
	}
}
