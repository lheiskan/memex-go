// Package claudecode is the Claude Code harness integration: where things go,
// and what the agent is told.
//
// It owns placement — skill directories, the SessionStart hook, the MCP
// registration — and static prose, the session-start directive. It does not
// own judgment, which is in skills/*/SKILL.md, and it does not compute facts
// about the store, which is package status's job.
package claudecode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"memex/internal/orderedjson"
)

// HookSubcommand is what the installed hook invokes.
const HookSubcommand = "harness session-start"

// MCPSubcommand is what Claude Code spawns for the MCP server.
const MCPSubcommand = "mcp"

// Action is what an install step did, for reporting.
type Action string

const (
	Installed Action = "installed"
	Updated   Action = "updated"
	Unchanged Action = "unchanged"
	WouldSet  Action = "would set"
)

// Step is one reported unit of work.
type Step struct {
	Action Action
	Detail string
}

// Options configures an install.
type Options struct {
	// ClaudeDir is Claude Code's config directory. Empty means ~/.claude.
	// Point it at a scratch copy to rehearse an install — which is the only
	// way it should ever be tested, since Claude Code blocks an agent from
	// editing the real settings file anyway.
	ClaudeDir string

	// HookCommand is the command the SessionStart hook runs. Empty means the
	// running executable plus " harness session-start". An absolute path is the point:
	// a hook runs in a non-interactive shell whose $PATH may not contain the
	// install prefix.
	HookCommand string

	// MCPCommand is the command Claude Code should spawn for the MCP server.
	// Empty means the running executable plus " mcp" — an absolute path that
	// needs no lookup, so it cannot rot the way a $PATH-resolved command can.
	MCPCommand string

	// DryRun writes nothing.
	DryRun bool

	// SkipMCP omits the `claude mcp add` step.
	SkipMCP bool
}

const backupSuffix = ".memex-bak"

// errNotJSON is the refusal shared by install and uninstall. A settings file
// we cannot read is never overwritten.
func errNotJSON(path string) error {
	return fmt.Errorf("%s is not valid JSON; refusing to modify it", path)
}

// SettingsPath returns the settings file an install would modify.
func SettingsPath(claudeDir string) (string, error) {
	dir, err := resolveDir(claudeDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

func resolveDir(claudeDir string) (string, error) {
	if claudeDir != "" {
		return claudeDir, nil
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, ".claude"), nil
}

// InstallSkills writes the embedded SKILL.md files under <claude-dir>/skills.
//
// skills is the embedded tree, keyed skills/<name>/SKILL.md. Only those named
// subdirectories are touched; nothing else under skills/ is read or removed.
func InstallSkills(skills fs, opts Options) ([]Step, error) {
	dir, err := resolveDir(opts.ClaudeDir)
	if err != nil {
		return nil, err
	}
	entries, err := skills.ReadDir("skills")
	if err != nil {
		return nil, err
	}
	var steps []Step
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		want, err := skills.ReadFile("skills/" + name + "/SKILL.md")
		if err != nil {
			return nil, err
		}
		dest := filepath.Join(dir, "skills", name, "SKILL.md")

		if got, err := os.ReadFile(dest); err == nil && string(got) == string(want) {
			steps = append(steps, Step{Unchanged, name})
			continue
		} else if err == nil {
			steps = append(steps, Step{actionOrWould(opts, Updated), name})
		} else {
			steps = append(steps, Step{actionOrWould(opts, Installed), name})
		}
		if opts.DryRun {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return nil, err
		}
		if err := writeFileAtomic(dest, want); err != nil {
			return nil, err
		}
	}
	return steps, nil
}

// fs is the subset of embed.FS this package needs, so tests can substitute a
// fixture without building one.
type fs interface {
	ReadDir(name string) ([]os.DirEntry, error)
	ReadFile(name string) ([]byte, error)
}

func actionOrWould(opts Options, a Action) Action {
	if opts.DryRun {
		return WouldSet
	}
	return a
}

// InstallHook merges a SessionStart group into settings.json, preserving
// everything it does not own byte-for-byte.
//
// Returns the step taken, plus any hand-written memex hook found. Such a hook
// is reported, never removed: deleting a user's own config without asking is
// not this tool's call.
func InstallHook(opts Options) (Step, []string, error) {
	path, err := SettingsPath(opts.ClaudeDir)
	if err != nil {
		return Step{}, nil, err
	}
	command := opts.HookCommand
	if command == "" {
		if command, err = selfCommand(HookSubcommand); err != nil {
			return Step{}, nil, err
		}
	}

	// Record existence before writing anything, so a fresh install does not
	// "back up" a settings file it just created itself.
	original, err := os.ReadFile(path)
	had := err == nil
	if !had {
		original = []byte("{}")
	}
	if !json.Valid(original) {
		return Step{}, nil, errNotJSON(path)
	}

	updated, foreign, err := mergeHook(original, command)
	if err != nil {
		return Step{}, nil, err
	}
	if string(updated) == string(original) {
		return Step{Unchanged, command}, foreign, nil
	}
	if opts.DryRun {
		return Step{WouldSet, command}, foreign, nil
	}
	if had {
		if _, statErr := os.Stat(path + backupSuffix); os.IsNotExist(statErr) {
			if err := writeFileAtomic(path+backupSuffix, original); err != nil {
				return Step{}, nil, err
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Step{}, nil, err
	}
	if err := writeFileAtomic(path, updated); err != nil {
		return Step{}, nil, err
	}
	return Step{Installed, command}, foreign, nil
}

// mergeHook rewrites only .hooks.SessionStart. Every other member of the file
// — and every group in SessionStart this tool did not write — is re-emitted
// from its original bytes.
func mergeHook(original []byte, command string) (out []byte, foreign []string, err error) {
	top, err := orderedjson.Decode(original)
	if err != nil {
		return nil, nil, err
	}

	events, groups, err := sessionStartGroups(top)
	if err != nil {
		return nil, nil, err
	}

	kept := make([]json.RawMessage, 0, len(groups)+1)
	for _, g := range groups {
		if isOurs(g) {
			continue // repoint rather than duplicate
		}
		if cmd, ok := foreignMemexCommand(g); ok {
			foreign = append(foreign, cmd)
		}
		kept = append(kept, g)
	}

	// A struct, not a map: encoding/json sorts map keys, which would emit
	// "hooks" before "matcher" and quietly differ from the shape every
	// existing settings file already has.
	ours, err := orderedjson.MarshalNoEscape(hookGroup{
		Matcher: "",
		Hooks:   []hookEntry{{Type: "command", Command: command}},
	}, 3)
	if err != nil {
		return nil, nil, err
	}
	kept = append(kept, ours)

	groupsRaw, err := encodeGroups(kept)
	if err != nil {
		return nil, nil, err
	}
	events = setPair(events, "SessionStart", groupsRaw)
	top = setPair(top, "hooks", orderedjson.Encode(events, 1))
	return append(orderedjson.Encode(top, 0), '\n'), foreign, nil
}

// encodeGroups lays out the SessionStart array, splicing each group's original
// bytes so a group this tool did not write keeps its member order and any
// fields this binary has never heard of (timeout, if, asyncRewake).
func encodeGroups(groups []json.RawMessage) (json.RawMessage, error) {
	if len(groups) == 0 {
		return json.RawMessage("[]"), nil
	}
	var b strings.Builder
	b.WriteString("[\n")
	for i, g := range groups {
		b.WriteString("      ")
		b.Write(reindent(g))
		if i < len(groups)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("    ]")
	return json.RawMessage(b.String()), nil
}

// reindent is a no-op placeholder kept explicit: group bytes are spliced as
// they arrived. Compacting or re-indenting them is what would churn a file we
// do not own.
func reindent(g json.RawMessage) []byte { return g }

func setPair(pairs []orderedjson.Pair, key string, raw json.RawMessage) []orderedjson.Pair {
	if i := orderedjson.Find(pairs, key); i >= 0 {
		pairs[i].Raw = raw
		return pairs
	}
	return append(pairs, orderedjson.Pair{Key: key, Raw: raw})
}

// hookGroup and hookEntry are the shape this tool writes. Field order is the
// emitted order.
type hookGroup struct {
	Matcher string      `json:"matcher"`
	Hooks   []hookEntry `json:"hooks"`
}

type hookEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// orderedDecode is orderedjson.Decode, named locally so check.go reads the
// same way as the merge path.
func orderedDecode(raw []byte) ([]orderedjson.Pair, error) { return orderedjson.Decode(raw) }

// sessionStartGroups walks a decoded settings object down to .hooks.SessionStart,
// returning the hooks object's members (so a caller can write one back) and the
// SessionStart groups. Missing keys are not an error: a settings file with no
// hooks at all is the fresh-install case.
func sessionStartGroups(top []orderedjson.Pair) ([]orderedjson.Pair, []json.RawMessage, error) {
	var events []orderedjson.Pair
	if i := orderedjson.Find(top, "hooks"); i >= 0 {
		var err error
		if events, err = orderedjson.Decode(top[i].Raw); err != nil {
			return nil, nil, err
		}
	}
	var groups []json.RawMessage
	if i := orderedjson.Find(events, "SessionStart"); i >= 0 {
		if err := json.Unmarshal(events[i].Raw, &groups); err != nil {
			return nil, nil, err
		}
	}
	return events, groups, nil
}

// probe is the minimum needed to classify a hook group. Groups are re-emitted
// from their original bytes, so unknown fields are never dropped.
type probe struct {
	Hooks []struct {
		Command string `json:"command"`
	} `json:"hooks"`
}

func commands(group json.RawMessage) []string {
	var p probe
	if json.Unmarshal(group, &p) != nil {
		return nil
	}
	out := make([]string, 0, len(p.Hooks))
	for _, h := range p.Hooks {
		out = append(out, strings.TrimSpace(h.Command))
	}
	return out
}

// isOurs reports whether a group was written by this tool.
//
// Matched on the command suffix rather than the whole string, so a moved
// binary is repointed on re-install rather than duplicated. The suffix is
// specific enough that it cannot claim someone else's hook.
func isOurs(group json.RawMessage) bool {
	for _, c := range commands(group) {
		if strings.HasSuffix(c, " "+HookSubcommand) {
			return true
		}
	}
	return false
}

// foreignMemexCommand reports a hand-written memex hook — one that mentions
// memex but is not ours. Such hooks are reported, never removed.
func foreignMemexCommand(group json.RawMessage) (string, bool) {
	if isOurs(group) {
		return "", false
	}
	for _, c := range commands(group) {
		if strings.Contains(strings.ToLower(c), "memex") {
			return c, true
		}
	}
	return "", false
}

// SelfCommand is selfCommand, exported for callers that need to name this
// binary in config they are writing (the MCP registration).
func SelfCommand(sub string) (string, error) { return selfCommand(sub) }

// selfCommand returns the running executable plus a subcommand, quoted if the
// path contains a space — a hook command is parsed by a shell.
func selfCommand(sub string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if abs, err := filepath.Abs(exe); err == nil {
		exe = abs
	}
	if strings.ContainsAny(exe, " \t") {
		return `"` + exe + `" ` + sub, nil
	}
	return exe + " " + sub, nil
}

// writeFileAtomic writes via temp + rename, as the card store does. On Windows
// os.Rename replaces an existing file (MOVEFILE_REPLACE_EXISTING), so this is
// not POSIX-only.
func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
