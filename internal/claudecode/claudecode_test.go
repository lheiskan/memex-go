package claudecode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"memex/internal/orderedjson"
)

// Each of these encodes a bug found end-to-end rather than by inspection,
// which is why they have survived three rewrites of the thing they test — a
// Go installer, a shell installer, and this one.

// fakeFS adapts fstest.MapFS to the fs interface this package consumes.
type fakeFS struct{ m fstest.MapFS }

func (f fakeFS) ReadDir(name string) ([]os.DirEntry, error) { return f.m.ReadDir(name) }
func (f fakeFS) ReadFile(name string) ([]byte, error)       { return f.m.ReadFile(name) }

func testSkills() fakeFS {
	return fakeFS{fstest.MapFS{
		"skills/memex-recall/SKILL.md":   {Data: []byte("recall body\n")},
		"skills/memex-retro/SKILL.md":    {Data: []byte("retro body\n")},
		"skills/memex-organize/SKILL.md": {Data: []byte("organize body\n")},
	}}
}

func settings(t *testing.T, dir string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("result is not valid JSON: %v\n%s", err, b)
	}
	return m
}

func hookCommands(t *testing.T, dir string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Hooks struct {
			SessionStart []probe `json:"SessionStart"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, g := range f.Hooks.SessionStart {
		for _, h := range g.Hooks {
			out = append(out, h.Command)
		}
	}
	return out
}

func TestFreshInstall(t *testing.T) {
	dir := t.TempDir()
	opts := Options{ClaudeDir: dir, HookCommand: "/bin/mx harness session-start"}

	steps, err := InstallSkills(testSkills(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 3 {
		t.Errorf("installed %d skills, want 3", len(steps))
	}
	for _, s := range steps {
		if s.Action != Installed {
			t.Errorf("skill %s: action %q, want %q", s.Detail, s.Action, Installed)
		}
	}
	if got, err := os.ReadFile(filepath.Join(dir, "skills", "memex-recall", "SKILL.md")); err != nil || string(got) != "recall body\n" {
		t.Errorf("skill content = %q, err %v", got, err)
	}

	step, foreign, err := InstallHook(opts)
	if err != nil {
		t.Fatal(err)
	}
	if step.Action != Installed {
		t.Errorf("hook action = %q, want %q", step.Action, Installed)
	}
	if len(foreign) != 0 {
		t.Errorf("foreign = %v, want none", foreign)
	}
	if got := hookCommands(t, dir); len(got) != 1 || got[0] != "/bin/mx harness session-start" {
		t.Errorf("commands = %v", got)
	}
	// No backup for a file that did not exist before we wrote it.
	if _, err := os.Stat(filepath.Join(dir, "settings.json"+backupSuffix)); !os.IsNotExist(err) {
		t.Error("backup written for a file we created ourselves")
	}
}

func TestReinstallIsANoOp(t *testing.T) {
	dir := t.TempDir()
	opts := Options{ClaudeDir: dir, HookCommand: "/bin/mx harness session-start"}
	if _, err := InstallSkills(testSkills(), opts); err != nil {
		t.Fatal(err)
	}
	if _, _, err := InstallHook(opts); err != nil {
		t.Fatal(err)
	}

	steps, err := InstallSkills(testSkills(), opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range steps {
		if s.Action != Unchanged {
			t.Errorf("skill %s: action %q, want %q", s.Detail, s.Action, Unchanged)
		}
	}
	step, _, err := InstallHook(opts)
	if err != nil {
		t.Fatal(err)
	}
	if step.Action != Unchanged {
		t.Errorf("hook action = %q, want %q", step.Action, Unchanged)
	}
}

func TestMovedBinaryIsRepointedNotDuplicated(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := InstallHook(Options{ClaudeDir: dir, HookCommand: "/old/mx harness session-start"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := InstallHook(Options{ClaudeDir: dir, HookCommand: "/new/mx harness session-start"}); err != nil {
		t.Fatal(err)
	}
	got := hookCommands(t, dir)
	if len(got) != 1 || got[0] != "/new/mx harness session-start" {
		t.Errorf("commands = %v, want exactly [/new/mcc session-start]", got)
	}
}

// Go's encoding/json rewrites < > & to \uXXXX by default, even inside a
// RawMessage, which would mangle every shell operator in a hook command this
// tool does not own.
func TestForeignContentSurvivesByteForByte(t *testing.T) {
	dir := t.TempDir()
	original := `{
  "model": "opus",
  "hooks": {
    "SessionStart": [
      {"matcher": "", "hooks": [{"type": "command", "command": "cat ~/n.md 2>/dev/null && echo '<ready>'"}]},
      {"matcher": "", "hooks": [{"type": "command", "command": "$HOME/.claude/hooks/session-start.sh"}], "timeout": 5}
    ],
    "Stop": [{"matcher": "", "hooks": [{"type": "command", "command": "echo done"}]}]
  },
  "env": {"FOO": "a&b"}
}
`
	path := filepath.Join(dir, "settings.json")
	write(t, path, original)

	if _, _, err := InstallHook(Options{ClaudeDir: dir, HookCommand: "/bin/mx harness session-start"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)

	for _, escaped := range []string{`\u003e`, `\u003c`, `\u0026`} {
		if strings.Contains(out, escaped) {
			t.Errorf("shell operators were re-escaped as %s:\n%s", escaped, out)
		}
	}
	for _, want := range []string{
		`"command": "cat ~/n.md 2>/dev/null && echo '<ready>'"`,
		`"FOO": "a&b"`,
		`"timeout": 5`,           // a field this binary has never heard of
		`"command": "echo done"`, // an event we do not touch
		`$HOME/.claude/hooks/session-start.sh`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("lost %q from the output:\n%s", want, out)
		}
	}

	// Top-level key order is the whole point of the ordered encoder: a
	// map round-trip would sort these alphabetically (env, hooks, model).
	if got := keyOrder(t, raw); got != "model,hooks,env" {
		t.Errorf("key order = %q, want %q", got, "model,hooks,env")
	}

	// The user's own session-start.sh must never be claimed.
	cmds := hookCommands(t, dir)
	if len(cmds) != 3 {
		t.Errorf("commands = %v, want 3 (two theirs, one ours)", cmds)
	}

	// The backup holds the original exactly.
	bak, err := os.ReadFile(path + backupSuffix)
	if err != nil {
		t.Fatal(err)
	}
	if string(bak) != original {
		t.Error("backup does not hold the original bytes")
	}
}

func TestHandWrittenMemexHookIsReportedNotRemoved(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "settings.json"),
		`{"hooks":{"SessionStart":[{"matcher":"","hooks":[{"type":"command","command":"memex sync && memex read index"}]}]}}`)

	_, foreign, err := InstallHook(Options{ClaudeDir: dir, HookCommand: "/bin/mx harness session-start"})
	if err != nil {
		t.Fatal(err)
	}
	if len(foreign) != 1 || !strings.Contains(foreign[0], "memex sync") {
		t.Errorf("foreign = %v, want the hand-written hook reported", foreign)
	}
	if got := hookCommands(t, dir); len(got) != 2 {
		t.Errorf("commands = %v, want theirs left in place alongside ours", got)
	}
}

func TestMalformedSettingsAreRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	write(t, path, "{not json")

	if _, _, err := InstallHook(Options{ClaudeDir: dir, HookCommand: "/bin/mx harness session-start"}); err == nil {
		t.Fatal("want an error, got nil")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "{not json" {
		t.Errorf("file was modified: %q (err %v)", got, err)
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	opts := Options{ClaudeDir: dir, HookCommand: "/bin/mx harness session-start", DryRun: true}

	if _, err := InstallSkills(testSkills(), opts); err != nil {
		t.Fatal(err)
	}
	if _, _, err := InstallHook(opts); err != nil {
		t.Fatal(err)
	}
	var files int
	filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			files++
		}
		return nil
	})
	if files != 0 {
		t.Errorf("dry run created %d files", files)
	}
}

func TestInstallIntoAnEmptyObjectCreatesOnlyHooks(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := InstallHook(Options{ClaudeDir: dir, HookCommand: "/bin/mx harness session-start"}); err != nil {
		t.Fatal(err)
	}
	m := settings(t, dir)
	if len(m) != 1 {
		t.Errorf("top-level keys = %v, want only hooks", m)
	}
}

// --- helpers ---

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustJSON(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func keyOrder(t *testing.T, raw []byte) string {
	t.Helper()
	pairs, err := orderedjson.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	keys := make([]string, len(pairs))
	for i, p := range pairs {
		keys[i] = p.Key
	}
	return strings.Join(keys, ",")
}
