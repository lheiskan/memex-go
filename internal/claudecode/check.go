package claudecode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// State is how one installed component compares to what install would write.
type State string

const (
	Current     State = "current"
	Stale       State = "stale"
	Absent      State = "not installed"
	Uncheckable State = "cannot check"
)

// Report answers "is my install current, and if not, why".
//
// The binary knows both what it would write and what is there, so it can say
// whether each skill is current, whether the hook still points at this
// executable, and whether the MCP server is registered by absolute path
// rather than through $PATH.
type Report struct {
	ConfigDir string
	Binary    string

	Skills []SkillReport
	Hook   HookReport
	MCP    MCPReport
}

type SkillReport struct {
	Name  string
	State State
}

type HookReport struct {
	State   State
	Command string // what is installed
	Want    string // what install would write
	Foreign []string
}

type MCPReport struct {
	State   State
	Command string
	Want    string
	Reason  string
}

// OK reports whether every component matches what install would write.
func (r Report) OK() bool {
	if r.Hook.State != Current {
		return false
	}
	if r.MCP.State != Current && r.MCP.State != Uncheckable {
		return false
	}
	for _, s := range r.Skills {
		if s.State != Current {
			return false
		}
	}
	return true
}

// Check inspects an install without modifying anything.
func Check(skills fs, opts Options) (Report, error) {
	dir, err := resolveDir(opts.ClaudeDir)
	if err != nil {
		return Report{}, err
	}
	self, err := os.Executable()
	if err != nil {
		return Report{}, err
	}
	if abs, aerr := filepath.Abs(self); aerr == nil {
		self = abs
	}
	r := Report{ConfigDir: dir, Binary: self}

	// --- skills: compare installed bytes against what we would write.
	entries, err := skills.ReadDir("skills")
	if err != nil {
		return Report{}, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		want, rerr := skills.ReadFile("skills/" + name + "/SKILL.md")
		if rerr != nil {
			return Report{}, rerr
		}
		got, gerr := os.ReadFile(filepath.Join(dir, "skills", name, "SKILL.md"))
		switch {
		case gerr != nil:
			r.Skills = append(r.Skills, SkillReport{name, Absent})
		case string(got) == string(want):
			r.Skills = append(r.Skills, SkillReport{name, Current})
		default:
			r.Skills = append(r.Skills, SkillReport{name, Stale})
		}
	}

	// --- hook.
	r.Hook.Want = opts.HookCommand
	if r.Hook.Want == "" {
		if r.Hook.Want, err = selfCommand(HookSubcommand); err != nil {
			return Report{}, err
		}
	}
	r.Hook.State, r.Hook.Command, r.Hook.Foreign, err =
		inspectHook(filepath.Join(dir, "settings.json"), r.Hook.Want)
	if err != nil {
		return Report{}, err
	}

	// --- mcp.
	r.MCP.Want = opts.MCPCommand
	if r.MCP.Want == "" {
		if r.MCP.Want, err = selfCommand(MCPSubcommand); err != nil {
			return Report{}, err
		}
	}
	// The MCP registration is global to the `claude` CLI, so it is not scoped
	// by --dir the way the settings file and skills are. --skip-mcp is how a
	// scratch check opts out of reading the real one.
	if opts.SkipMCP {
		r.MCP = MCPReport{State: Uncheckable, Want: r.MCP.Want, Reason: "skipped (--skip-mcp)"}
	} else {
		r.MCP = inspectMCP(r.MCP.Want)
	}
	return r, nil
}

func inspectHook(path, want string) (State, string, []string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Absent, "", nil, nil
	}
	if !json.Valid(raw) {
		return Uncheckable, "", nil, fmt.Errorf("%s is not valid JSON", path)
	}
	top, err := orderedDecode(raw)
	if err != nil {
		return Uncheckable, "", nil, err
	}
	_, groups, err := sessionStartGroups(top)
	if err != nil {
		return Uncheckable, "", nil, err
	}

	var foreign []string
	for _, g := range groups {
		if isOurs(g) {
			cmd := ourCommand(g)
			if cmd == want {
				return Current, cmd, foreign, nil
			}
			return Stale, cmd, foreign, nil
		}
		if cmd, ok := foreignMemexCommand(g); ok {
			foreign = append(foreign, cmd)
		}
	}
	return Absent, "", foreign, nil
}

// ourCommand returns the hook command this tool wrote, from a group isOurs
// has already matched.
func ourCommand(group json.RawMessage) string {
	for _, c := range commands(group) {
		if strings.HasSuffix(c, " "+HookSubcommand) {
			return c
		}
	}
	return ""
}

func inspectMCP(want string) MCPReport {
	got, found, checkable := mcpRegistration()
	switch {
	case !checkable:
		return MCPReport{State: Uncheckable, Want: want, Reason: "the claude CLI is not on $PATH"}
	case !found:
		return MCPReport{State: Absent, Want: want}
	case got == want:
		return MCPReport{State: Current, Command: got, Want: want}
	}
	reason := "registered to a different path"
	// A bare command is the interesting case: it resolves through $PATH at
	// spawn time, which is what rotted once before.
	if fields := strings.Fields(got); len(fields) > 0 && !filepath.IsAbs(strings.Trim(fields[0], `"`)) {
		reason = "registered via $PATH rather than an absolute path"
	}
	return MCPReport{State: Stale, Command: got, Want: want, Reason: reason}
}

// FormatReport renders a Report. The wording lives here rather than in the
// engine because it does not vary with the card store.
func FormatReport(r Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "harness       claude-code\n")
	fmt.Fprintf(&b, "config dir    %s\n", r.ConfigDir)
	fmt.Fprintf(&b, "binary        %s\n\n", r.Binary)

	current, stale, absent := 0, 0, 0
	for _, s := range r.Skills {
		switch s.State {
		case Current:
			current++
		case Stale:
			stale++
		default:
			absent++
		}
	}
	fmt.Fprintf(&b, "skills        %s\n", counts(current, stale, absent))
	for _, s := range r.Skills {
		if s.State != Current {
			fmt.Fprintf(&b, "              %-26s %s\n", s.Name, s.State)
		}
	}

	fmt.Fprintf(&b, "session hook  %s\n", r.Hook.State)
	if r.Hook.State != Current {
		if r.Hook.Command != "" {
			fmt.Fprintf(&b, "              installed: %s\n", r.Hook.Command)
		}
		fmt.Fprintf(&b, "              expected:  %s\n", r.Hook.Want)
	}
	for _, f := range r.Hook.Foreign {
		fmt.Fprintf(&b, "              also present (not ours, left alone): %s\n", f)
	}

	fmt.Fprintf(&b, "mcp server    %s\n", r.MCP.State)
	if r.MCP.State != Current {
		if r.MCP.Reason != "" {
			fmt.Fprintf(&b, "              %s\n", r.MCP.Reason)
		}
		if r.MCP.Command != "" {
			fmt.Fprintf(&b, "              installed: %s\n", r.MCP.Command)
		}
		if r.MCP.State != Uncheckable {
			fmt.Fprintf(&b, "              expected:  %s\n", r.MCP.Want)
		}
	}

	if !r.OK() {
		b.WriteString("\nrun `memex harness install` to bring this up to date\n")
	}
	return b.String()
}

func counts(current, stale, absent int) string {
	parts := []string{fmt.Sprintf("%d current", current)}
	if stale > 0 {
		parts = append(parts, fmt.Sprintf("%d stale", stale))
	}
	if absent > 0 {
		parts = append(parts, fmt.Sprintf("%d missing", absent))
	}
	return strings.Join(parts, ", ")
}
