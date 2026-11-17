package claudecode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isOurs matches on the command suffix, which is what makes a re-install
// repoint a moved binary instead of appending a second hook. The cases that
// must NOT match are the valuable half: claiming someone else's SessionStart
// hook would be destructive.
func TestIsOursMatchesOnlyOurHook(t *testing.T) {
	tests := []struct {
		command string
		want    bool
	}{
		{"/Users/x/go/bin/memex harness session-start", true},
		{`"/Users/a b/go/bin/memex" harness session-start`, true},
		{"/usr/local/bin/memex harness session-start", true},

		// Not ours, and must never be claimed.
		{"/opt/other-tool/hooks/session-start.sh", false},
		{"$HOME/.claude/hooks/session-start.sh", false},
		{"/old/bin/memex session-start", false},
		{"memex sync && memex read index", false},
		{"cat ~/notes.md", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			group := []byte(`{"matcher":"","hooks":[{"type":"command","command":` +
				mustJSON(t, tt.command) + `}]}`)
			if got := isOurs(group); got != tt.want {
				t.Errorf("isOurs(%q) = %v, want %v", tt.command, got, tt.want)
			}
		})
	}
}

func seedHook(t *testing.T, dir, command string) {
	t.Helper()
	write(t, filepath.Join(dir, "settings.json"),
		`{"hooks":{"SessionStart":[{"matcher":"","hooks":[{"type":"command","command":`+
			mustJSON(t, command)+`}]}]}}`)
}

func TestCheckReportsEachState(t *testing.T) {
	want := "/bin/mx harness session-start"

	t.Run("absent", func(t *testing.T) {
		dir := t.TempDir()
		r := checkIn(t, dir, want)
		if r.Hook.State != Absent {
			t.Errorf("hook state = %q, want %q", r.Hook.State, Absent)
		}
		if r.OK() {
			t.Error("OK() = true with nothing installed")
		}
	})

	t.Run("current", func(t *testing.T) {
		dir := t.TempDir()
		seedHook(t, dir, want)
		r := checkIn(t, dir, want)
		if r.Hook.State != Current {
			t.Errorf("hook state = %q, want %q", r.Hook.State, Current)
		}
	})

	// A moved binary. The interesting case, because a naive "is a hook of
	// ours present" check would call this current.
	t.Run("stale by path", func(t *testing.T) {
		dir := t.TempDir()
		seedHook(t, dir, "/elsewhere/mx harness session-start")
		r := checkIn(t, dir, want)
		if r.Hook.State != Stale {
			t.Errorf("hook state = %q, want %q", r.Hook.State, Stale)
		}
		out := FormatReport(r)
		if !strings.Contains(out, "installed: /elsewhere/mx harness session-start") {
			t.Errorf("report should show what is installed:\n%s", out)
		}
		if !strings.Contains(out, "expected:  "+want) {
			t.Errorf("report should show what is expected:\n%s", out)
		}
	})
}

// A hand-written memex hook is reported by check, as it is by install, and
// never counted as ours.
func TestCheckReportsForeignMemexHook(t *testing.T) {
	dir := t.TempDir()
	seedHook(t, dir, "memex sync && memex read index")
	r := checkIn(t, dir, "/bin/mx harness session-start")

	if r.Hook.State != Absent {
		t.Errorf("hook state = %q, want %q — a foreign hook is not ours", r.Hook.State, Absent)
	}
	if len(r.Hook.Foreign) != 1 {
		t.Fatalf("foreign = %v, want the hand-written hook", r.Hook.Foreign)
	}
	if !strings.Contains(FormatReport(r), "left alone") {
		t.Errorf("report should say the foreign hook was left alone:\n%s", FormatReport(r))
	}
}

func TestCheckSkillStates(t *testing.T) {
	dir := t.TempDir()
	opts := Options{ClaudeDir: dir, HookCommand: "/bin/mx harness session-start", SkipMCP: true}
	if _, err := InstallSkills(testSkills(), opts); err != nil {
		t.Fatal(err)
	}
	// Edit one, delete another.
	write(t, filepath.Join(dir, "skills", "memex-retro", "SKILL.md"), "hand-edited\n")
	if err := os.Remove(filepath.Join(dir, "skills", "memex-organize", "SKILL.md")); err != nil {
		t.Fatal(err)
	}

	r, err := Check(testSkills(), opts)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]State{}
	for _, s := range r.Skills {
		got[s.Name] = s.State
	}
	for name, want := range map[string]State{
		"memex-recall":   Current,
		"memex-retro":    Stale,
		"memex-organize": Absent,
	} {
		if got[name] != want {
			t.Errorf("%s = %q, want %q", name, got[name], want)
		}
	}
	if out := FormatReport(r); !strings.Contains(out, "1 current, 1 stale, 1 missing") {
		t.Errorf("counts line wrong:\n%s", out)
	}
}

// --skip-mcp is how a scratch check opts out of reading the real `claude`
// config, which is global and not scoped by --dir.
func TestCheckSkipMCPDoesNotExecClaude(t *testing.T) {
	dir := t.TempDir()
	want := "/bin/mx harness session-start"
	// A fully current install apart from the MCP registration, so OK() can
	// only be false because of the thing being skipped.
	if _, err := InstallSkills(testSkills(), Options{ClaudeDir: dir}); err != nil {
		t.Fatal(err)
	}
	seedHook(t, dir, want)
	r := checkIn(t, dir, want)

	if r.MCP.State != Uncheckable {
		t.Errorf("mcp state = %q, want %q", r.MCP.State, Uncheckable)
	}
	// An unchecked MCP registration must not make OK() false, or every
	// scratch check would report a problem it did not look for.
	if !r.OK() {
		t.Error("OK() = false when only the MCP check was skipped")
	}
}

func TestUninstallRemovesOnlyOurs(t *testing.T) {
	dir := t.TempDir()
	original := `{
  "model": "opus",
  "hooks": {
    "SessionStart": [
      {"matcher": "", "hooks": [{"type": "command", "command": "/other/tool.sh"}], "timeout": 5}
    ]
  },
  "env": {"FOO": "a&b"}
}
`
	path := filepath.Join(dir, "settings.json")
	write(t, path, original)
	opts := Options{ClaudeDir: dir, HookCommand: "/bin/mx harness session-start", SkipMCP: true}
	if _, err := InstallSkills(testSkills(), opts); err != nil {
		t.Fatal(err)
	}
	if _, _, err := InstallHook(opts); err != nil {
		t.Fatal(err)
	}

	steps, _, err := Uninstall(testSkills(), opts)
	if err != nil {
		t.Fatal(err)
	}
	var removed int
	for _, s := range steps {
		if s.Action == Removed {
			removed++
		}
	}
	if removed != 4 { // three skills plus the hook
		t.Errorf("removed %d things, want 4", removed)
	}

	cmds := hookCommands(t, dir)
	if len(cmds) != 1 || cmds[0] != "/other/tool.sh" {
		t.Errorf("commands = %v, want only the foreign hook", cmds)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"model": "opus"`, `"FOO": "a&b"`, `"timeout": 5`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("uninstall lost %q:\n%s", want, raw)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "skills", "memex-recall", "SKILL.md")); !os.IsNotExist(err) {
		t.Error("skill file survived uninstall")
	}
}

func TestUninstallDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	opts := Options{ClaudeDir: dir, HookCommand: "/bin/mx harness session-start", SkipMCP: true}
	if _, err := InstallSkills(testSkills(), opts); err != nil {
		t.Fatal(err)
	}
	if _, _, err := InstallHook(opts); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}

	opts.DryRun = true
	if _, _, err := Uninstall(testSkills(), opts); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("dry-run uninstall modified settings.json")
	}
	if _, err := os.Stat(filepath.Join(dir, "skills", "memex-recall", "SKILL.md")); err != nil {
		t.Error("dry-run uninstall removed a skill")
	}
}

func checkIn(t *testing.T, dir, want string) Report {
	t.Helper()
	r, err := Check(testSkills(), Options{ClaudeDir: dir, HookCommand: want, SkipMCP: true})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
