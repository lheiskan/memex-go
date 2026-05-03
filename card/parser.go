package card

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Frontmatter is an ordered key→value collection. Order is preserved so that
// round-tripping a card produces byte-identical output (no gratuitous git diffs).
type Frontmatter struct {
	Fields []Field
}

type Field struct {
	Key   string
	Value any
}

func (fm *Frontmatter) Get(key string) (any, bool) {
	for _, f := range fm.Fields {
		if f.Key == key {
			return f.Value, true
		}
	}
	return nil, false
}

func (fm *Frontmatter) GetString(key string) string {
	v, ok := fm.Get(key)
	if !ok {
		return ""
	}
	return valueToString(v)
}

// Set updates an existing field in place or appends a new one at the end.
func (fm *Frontmatter) Set(key string, value any) {
	for i := range fm.Fields {
		if fm.Fields[i].Key == key {
			fm.Fields[i].Value = value
			return
		}
	}
	fm.Fields = append(fm.Fields, Field{Key: key, Value: value})
}

func (fm *Frontmatter) Delete(key string) {
	for i, f := range fm.Fields {
		if f.Key == key {
			fm.Fields = append(fm.Fields[:i], fm.Fields[i+1:]...)
			return
		}
	}
}

// ParseFrontmatter splits a card file into frontmatter + body.
//
// Mirrors the JS reference (lib/parser.js): if the YAML block is malformed,
// fall back to empty metadata + everything-after-the-block as body. If there
// is no leading "---", the whole file is body.
func ParseFrontmatter(raw []byte) (Frontmatter, []byte) {
	if !bytes.HasPrefix(raw, []byte("---\n")) && !bytes.HasPrefix(raw, []byte("---\r\n")) {
		return Frontmatter{}, raw
	}

	// Skip the opening delimiter line.
	rest := raw[4:]
	if bytes.HasPrefix(raw, []byte("---\r\n")) {
		rest = raw[5:]
	}

	// Find the closing delimiter: a line containing only "---" (optionally with \r).
	closeIdx := findClosingDelimiter(rest)
	if closeIdx < 0 {
		// No closing delimiter — treat whole file as body, like gray-matter.
		return Frontmatter{}, raw
	}

	yamlBytes := rest[:closeIdx]
	body := rest[closeIdx:]
	body = stripClosingDelimiter(body)

	fm, err := parseYAMLOrdered(yamlBytes)
	if err != nil {
		// Malformed YAML — match the JS fallback: empty metadata, body is whatever
		// follows the bad block (the regex strip in parser.js). Note: JS uses regex
		// /^---[\s\S]*?---\n?/ which finds the first "---" anywhere on a line; we
		// already located the closing delimiter, so body is correct.
		return Frontmatter{}, body
	}
	return fm, body
}

// findClosingDelimiter returns the byte offset of the first line containing
// only "---", or -1 if none exists. The returned offset points at the start
// of the "---" line.
func findClosingDelimiter(b []byte) int {
	offset := 0
	for offset < len(b) {
		// Find the next newline.
		nl := bytes.IndexByte(b[offset:], '\n')
		var line []byte
		var lineStart = offset
		if nl < 0 {
			line = b[offset:]
			offset = len(b)
		} else {
			line = b[offset : offset+nl]
			offset += nl + 1
		}
		// Trim trailing CR for Windows line endings.
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		if string(line) == "---" {
			return lineStart
		}
	}
	return -1
}

// stripClosingDelimiter removes the leading "---" line (and its newline) from b.
func stripClosingDelimiter(b []byte) []byte {
	// b begins with "---" possibly followed by \r\n or \n or EOF.
	if !bytes.HasPrefix(b, []byte("---")) {
		return b
	}
	rest := b[3:]
	if bytes.HasPrefix(rest, []byte("\r\n")) {
		return rest[2:]
	}
	if bytes.HasPrefix(rest, []byte("\n")) {
		return rest[1:]
	}
	return rest
}

// parseYAMLOrdered decodes a YAML mapping into an ordered Frontmatter using
// yaml.v3's Node API. Only top-level mappings are supported (matches memex).
func parseYAMLOrdered(b []byte) (Frontmatter, error) {
	if len(bytes.TrimSpace(b)) == 0 {
		return Frontmatter{}, nil
	}
	var root yaml.Node
	if err := yaml.Unmarshal(b, &root); err != nil {
		return Frontmatter{}, err
	}
	// DocumentNode → MappingNode
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return Frontmatter{}, nil
	}
	mapping := root.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return Frontmatter{}, fmt.Errorf("frontmatter is not a YAML mapping")
	}
	fm := Frontmatter{}
	// Mapping content is alternating key/value nodes.
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		keyNode := mapping.Content[i]
		valNode := mapping.Content[i+1]
		var v any
		if err := valNode.Decode(&v); err != nil {
			// Fall back to raw value string for this field.
			v = valNode.Value
		}
		fm.Fields = append(fm.Fields, Field{Key: keyNode.Value, Value: v})
	}
	return fm, nil
}

// specialChars triggers single-quoting in StringifyFrontmatter. Mirrors the
// regex in lib/parser.js: [:#{}[\],&*?|>!%@`']
var specialChars = regexp.MustCompile("[:#{}\\[\\],&*?|>!%@`']")

// StringifyFrontmatter emits a card file. Hand-rolled (does NOT use yaml.v3's
// encoder) to guarantee byte-identical output with the upstream JS reference,
// avoiding js-yaml block scalars and gopkg.in/yaml.v3 quirks.
//
//   - skips nil values entirely
//   - flattens \n in values to spaces, then trim
//   - empty or special-char-bearing strings → single-quoted, ' doubled
//   - everything else → bare
func StringifyFrontmatter(body []byte, fm Frontmatter) []byte {
	var lines []string
	for _, f := range fm.Fields {
		if f.Value == nil {
			continue
		}
		s := valueToString(f.Value)
		s = strings.ReplaceAll(s, "\n", " ")
		s = strings.TrimSpace(s)
		var line string
		if s == "" || specialChars.MatchString(s) {
			line = fmt.Sprintf("%s: '%s'", f.Key, strings.ReplaceAll(s, "'", "''"))
		} else {
			line = fmt.Sprintf("%s: %s", f.Key, s)
		}
		lines = append(lines, line)
	}
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.WriteString(strings.Join(lines, "\n"))
	buf.WriteString("\n---\n")
	buf.Write(body)
	return buf.Bytes()
}

// valueToString mirrors JavaScript's String(value) for the value types we see
// in card frontmatter, with one deliberate deviation: time.Time is normalized
// to an ISO short date (YYYY-MM-DD) rather than JS's verbose Date.toString().
// This matches lib/commands/write.js's coercion of `created` to an ISO string
// and avoids the legacy long-form garbage seen in older cards.
func valueToString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		// Match JS Number.toString(): integer-valued floats print without ".0"
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'g', -1, 64)
	case time.Time:
		return x.Format("2006-01-02")
	case []any:
		// Mirrors String([a, b, c]) → "a,b,c". Arrays lose their structure here,
		// matching the upstream JS behavior. search.go must accept both forms.
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = valueToString(e)
		}
		return strings.Join(parts, ",")
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ExtractLinks returns the unique [[wikilink]] targets in body, in first-seen order.
func ExtractLinks(body []byte) []string {
	re := regexp.MustCompile(`\[\[([^\]]+)\]\]`)
	matches := re.FindAllSubmatch(body, -1)
	seen := make(map[string]struct{}, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		s := string(m[1])
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
