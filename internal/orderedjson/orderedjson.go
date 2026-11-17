// Package orderedjson edits a JSON object without reordering or re-escaping
// the parts it was not asked to change.
//
// Roughly 90 lines, written once, withdrawn, and revived unchanged. `jq` has
// both properties below by construction, which is why the installer could
// drop this code while it was a shell script; Go has neither, so moving the
// installer back into Go needed it again.
//
// Two failures it exists to prevent, both in a file the user maintains by
// hand:
//
//   - Decoding to map[string]any and re-marshalling sorts every key
//     alphabetically. This project's position on that is settled: do not
//     rewrite a file the user maintains by hand just because re-emitting it
//     was easier.
//   - encoding/json escapes <, > and & to \uXXXX by default, and does it even
//     to bytes inside a json.RawMessage. Left on, every shell operator in a
//     neighbouring hook command gets rewritten: 2>/dev/null comes back as
//     2\u003e/dev/null and && as \u0026\u0026, which is semantically
//     identical and alarming to read.
package orderedjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Pair is one member of a JSON object, carrying the value's original bytes so
// untouched members can be re-emitted verbatim.
type Pair struct {
	Key string
	Raw json.RawMessage
}

// Decode reads a JSON object into its members, in file order.
func Decode(data []byte) ([]Pair, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("expected a JSON object, got %v", tok)
	}
	var pairs []Pair
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("expected an object key, got %v", tok)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		pairs = append(pairs, Pair{Key: key, Raw: raw})
	}
	if _, err := dec.Token(); err != nil { // closing brace
		return nil, err
	}
	return pairs, nil
}

// Encode re-emits members at the given depth, two-space indented. Raw values
// are spliced in unchanged, which keeps members this tool did not touch
// byte-identical to the input.
func Encode(pairs []Pair, depth int) []byte {
	outer := strings.Repeat("  ", depth)
	inner := strings.Repeat("  ", depth+1)
	if len(pairs) == 0 {
		return []byte("{}")
	}
	var b bytes.Buffer
	b.WriteString("{\n")
	for i, p := range pairs {
		key, _ := MarshalNoEscape(p.Key, 0)
		b.WriteString(inner)
		b.Write(key)
		b.WriteString(": ")
		b.Write(p.Raw)
		if i < len(pairs)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString(outer)
	b.WriteString("}")
	return b.Bytes()
}

// MarshalNoEscape encodes v without Go's default HTML escaping of < > &.
// depth is the indent prefix applied to every line but the first.
func MarshalNoEscape(v any, depth int) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent(strings.Repeat("  ", depth), "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// Find returns the index of key in pairs, or -1.
func Find(pairs []Pair, key string) int {
	for i, p := range pairs {
		if p.Key == key {
			return i
		}
	}
	return -1
}
