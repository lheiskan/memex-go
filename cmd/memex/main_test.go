package main

import (
	"flag"
	"strings"
	"testing"
)

// The case that motivated parseFlags: with the stdlib's own Parse, the query
// becomes "docker --limit 3" and the search returns nothing, which is
// indistinguishable from an empty memory.
func TestFlagsAfterQueryAreStillFlags(t *testing.T) {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	limit := fs.Int("limit", 10, "")
	pos := parseFlags(fs, []string{"docker", "--limit", "3"})

	if got := strings.Join(pos, " "); got != "docker" {
		t.Errorf("query = %q, want %q", got, "docker")
	}
	if *limit != 3 {
		t.Errorf("limit = %d, want 3", *limit)
	}
}

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		want  string // positionals, space-joined
		limit int
		tag   string
	}{
		{"flags first", []string{"--limit", "3", "docker"}, "docker", 3, ""},
		{"flags last", []string{"docker", "--limit", "3"}, "docker", 3, ""},
		{"flags both sides", []string{"--tag", "go", "docker", "--limit", "3"}, "docker", 3, "go"},
		{"interleaved positionals", []string{"go", "--limit", "2", "docker"}, "go docker", 2, ""},
		{"equals form", []string{"docker", "--limit=7"}, "docker", 7, ""},
		{"no flags", []string{"docker", "compose"}, "docker compose", 10, ""},
		{"no args", nil, "", 10, ""},
		{"only flags", []string{"--limit", "1"}, "", 1, ""},
		{"dash terminator keeps dashed token", []string{"--limit", "5", "--", "--limit"}, "--limit", 5, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := flag.NewFlagSet("search", flag.ContinueOnError)
			limit := fs.Int("limit", 10, "")
			tag := fs.String("tag", "", "")
			pos := parseFlags(fs, tt.args)

			if got := strings.Join(pos, " "); got != tt.want {
				t.Errorf("positionals = %q, want %q", got, tt.want)
			}
			if *limit != tt.limit {
				t.Errorf("limit = %d, want %d", *limit, tt.limit)
			}
			if *tag != tt.tag {
				t.Errorf("tag = %q, want %q", *tag, tt.tag)
			}
		})
	}
}

// --target exists so the set of harnesses is discoverable in `harness help`
// and enforced in the error path, rather than asserted in a prose sentence
// that goes stale when the set changes. These assert the two cannot drift.
func TestValidateTarget(t *testing.T) {
	for _, name := range supportedHarnesses {
		if err := validateTarget(name); err != nil {
			t.Errorf("validateTarget(%q) = %v, want nil — it is in supportedHarnesses", name, err)
		}
	}
	if err := validateTarget(defaultHarness); err != nil {
		t.Errorf("the default %q is not accepted: %v", defaultHarness, err)
	}
	for _, name := range []string{"cursor", "codex", "", "Claude-Code", "claude code"} {
		err := validateTarget(name)
		if err == nil {
			t.Errorf("validateTarget(%q) = nil, want an error", name)
			continue
		}
		// The error has to name the alternatives, or it tells the user
		// nothing they can act on.
		for _, h := range supportedHarnesses {
			if !strings.Contains(err.Error(), h) {
				t.Errorf("error for %q does not list %q: %v", name, h, err)
			}
		}
	}
}

func TestSupportedListReadsCorrectly(t *testing.T) {
	got := supportedList()
	for _, h := range supportedHarnesses {
		if !strings.Contains(got, h) {
			t.Errorf("supportedList() = %q, missing %q", got, h)
		}
	}
	// With one value, naming a default separately is redundant noise.
	if len(supportedHarnesses) == 1 && strings.Contains(got, ", default ") {
		t.Errorf("supportedList() = %q, want no separate default for a single value", got)
	}
}
