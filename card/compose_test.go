package card

import "testing"

const today = "2026-11-17"

func TestComposeAuthorsCanonicalFieldOrder(t *testing.T) {
	out, err := Compose([]byte("An insight."), nil, AuthorOptions{
		Title:         "A New Card",
		Category:      "tooling",
		Tags:          "go,cli",
		DefaultSource: "claude-code",
		Today:         today,
	})
	if err != nil {
		t.Fatal(err)
	}

	// The order the store's existing cards are already in. A different order
	// would be correct YAML and still churn every card on next touch.
	want := "---\ntitle: A New Card\ncreated: 2026-11-17\nsource: claude-code\n" +
		"category: tooling\ntags: 'go,cli'\nmodified: 2026-11-17\n---\n\nAn insight.\n"
	if string(out) != want {
		t.Errorf("Compose() =\n%q\nwant\n%q", out, want)
	}
}

func TestComposeRequiresATitleItCannotInvent(t *testing.T) {
	if _, err := Compose([]byte("Body only."), nil, AuthorOptions{Today: today}); err == nil {
		t.Fatal("Compose() with no title and no existing card: want error, got nil")
	}
}

// The read-append-write idiom must not alter a byte beyond `modified`, or
// every card churns the store's git history on next touch.
func TestComposePassesSuppliedFrontmatterThrough(t *testing.T) {
	existing := []byte("---\ntitle: Kept\ncreated: 2026-01-02\nsource: manual\n" +
		"tags: 'a,b'\nmodified: 2026-01-02\n---\n\nOriginal body.\n")

	out, err := Compose(existing, existing, AuthorOptions{
		DefaultSource: "claude-code",
		Today:         today,
	})
	if err != nil {
		t.Fatal(err)
	}

	want := "---\ntitle: Kept\ncreated: 2026-01-02\nsource: manual\n" +
		"tags: 'a,b'\nmodified: 2026-11-17\n---\n\nOriginal body.\n"
	if string(out) != want {
		t.Errorf("Compose() =\n%q\nwant\n%q", out, want)
	}
}

// A bare body against an existing card keeps its metadata: a body rewrite must
// not silently reset the card's created date to today.
func TestComposeInheritsExistingFrontmatterForABareBody(t *testing.T) {
	existing := []byte("---\ntitle: Kept\ncreated: 2026-01-02\nsource: manual\n" +
		"category: old\nmodified: 2026-01-02\n---\n\nOriginal body.\n")

	out, err := Compose([]byte("Replaced body."), existing, AuthorOptions{
		DefaultSource: "claude-code",
		Today:         today,
	})
	if err != nil {
		t.Fatal(err)
	}

	fm, body := ParseFrontmatter(out)
	if got := fm.GetString("created"); got != "2026-01-02" {
		t.Errorf("created = %q, want it preserved as 2026-01-02", got)
	}
	if got := fm.GetString("source"); got != "manual" {
		t.Errorf("source = %q, want the existing %q to survive the default", got, "manual")
	}
	if got := fm.GetString("modified"); got != today {
		t.Errorf("modified = %q, want %q", got, today)
	}
	if string(body) != "\nReplaced body.\n" {
		t.Errorf("body = %q, want %q", body, "\nReplaced body.\n")
	}
}

func TestComposeSourcePrecedence(t *testing.T) {
	tests := []struct {
		name     string
		incoming string
		opt      AuthorOptions
		want     string
	}{
		{
			"explicit source overrides frontmatter",
			"---\ntitle: T\ncreated: 2026-01-02\nsource: manual\n---\n\nB.\n",
			AuthorOptions{Source: "organize", DefaultSource: "claude-code", Today: today},
			"organize",
		},
		{
			"default fills only a gap",
			"---\ntitle: T\ncreated: 2026-01-02\nsource: manual\n---\n\nB.\n",
			AuthorOptions{DefaultSource: "claude-code", Today: today},
			"manual",
		},
		{
			"default applies to a new card",
			"B.",
			AuthorOptions{Title: "T", DefaultSource: "claude-code", Today: today},
			"claude-code",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := Compose([]byte(tt.incoming), nil, tt.opt)
			if err != nil {
				t.Fatal(err)
			}
			fm, _ := ParseFrontmatter(out)
			if got := fm.GetString("source"); got != tt.want {
				t.Errorf("source = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestComposeNormalizesAuthoredBodyLayout(t *testing.T) {
	for _, in := range []string{"Body.", "Body.\n", "\nBody.\n", "\n\nBody.\n"} {
		out, err := Compose([]byte(in), nil, AuthorOptions{Title: "T", Today: today})
		if err != nil {
			t.Fatal(err)
		}
		_, body := ParseFrontmatter(out)
		if len(body) == 0 || body[0] != '\n' {
			t.Errorf("Compose(%q): body must start with a blank line, got %q", in, body)
		}
		if body[len(body)-1] != '\n' {
			t.Errorf("Compose(%q): body must end with a newline, got %q", in, body)
		}
	}
}
