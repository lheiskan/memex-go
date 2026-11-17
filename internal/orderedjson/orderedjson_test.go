package orderedjson

import (
	"encoding/json"
	"strings"
	"testing"
)

// The baseline this package exists to beat. If Go ever stops sorting map keys
// or stops HTML-escaping by default, this test fails and the package's reason
// for existing should be re-examined rather than assumed.
func TestStdlibStillHasBothProblems(t *testing.T) {
	in := []byte(`{"model":"opus","hooks":{},"env":{"FOO":"a&b"}}`)

	var m map[string]any
	if err := json.Unmarshal(in, &m); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(string(out), `{"env"`) {
		t.Errorf("expected a map round-trip to sort keys, got %s", out)
	}
	if !strings.Contains(string(out), `\u0026`) {
		t.Errorf("expected default HTML escaping of & to \\u0026, got %s", out)
	}
}

func TestDecodePreservesFileOrder(t *testing.T) {
	// Deliberately neither alphabetical nor reverse: this is the shape of a
	// hand-maintained settings file.
	in := []byte(`{"env":{},"permissions":{},"hooks":{},"effortLevel":"high","voice":false}`)

	pairs, err := Decode(in)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, p := range pairs {
		keys = append(keys, p.Key)
	}
	want := "env,permissions,hooks,effortLevel,voice"
	if got := strings.Join(keys, ","); got != want {
		t.Errorf("keys = %q, want %q", got, want)
	}
}

func TestRoundTripIsByteIdenticalApartFromIndentation(t *testing.T) {
	in := []byte(`{"a":1,"b":{"nested":true},"c":[1,2,3]}`)
	pairs, err := Decode(in)
	if err != nil {
		t.Fatal(err)
	}
	out := Encode(pairs, 0)

	// Values are spliced from their original bytes, so they survive verbatim.
	for _, want := range []string{`"a": 1`, `"b": {"nested":true}`, `"c": [1,2,3]`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	var check map[string]any
	if err := json.Unmarshal(out, &check); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
}

func TestMarshalNoEscapeLeavesShellOperatorsAlone(t *testing.T) {
	out, err := MarshalNoEscape("cat x 2>/dev/null && echo '<ready>'", 0)
	if err != nil {
		t.Fatal(err)
	}
	want := `"cat x 2>/dev/null && echo '<ready>'"`
	if string(out) != want {
		t.Errorf("MarshalNoEscape = %s, want %s", out, want)
	}
}

func TestDecodeRejectsNonObjects(t *testing.T) {
	for _, in := range []string{`[1,2]`, `"a string"`, `42`, ``} {
		if _, err := Decode([]byte(in)); err == nil {
			t.Errorf("Decode(%q): want an error, got nil", in)
		}
	}
}

func TestEncodeEmptyObject(t *testing.T) {
	if got := string(Encode(nil, 0)); got != "{}" {
		t.Errorf("Encode(nil) = %q, want %q", got, "{}")
	}
}

func TestFind(t *testing.T) {
	pairs := []Pair{{Key: "a"}, {Key: "b"}}
	if got := Find(pairs, "b"); got != 1 {
		t.Errorf("Find(b) = %d, want 1", got)
	}
	if got := Find(pairs, "missing"); got != -1 {
		t.Errorf("Find(missing) = %d, want -1", got)
	}
}
