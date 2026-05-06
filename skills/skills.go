// Package skills embeds the Claude Code SKILL.md files that ship alongside
// the binary and writes them out to a target directory (typically
// ~/.claude/skills/) on demand. This is the bundled equivalent of the upstream
// npm package's "copy skills/ from node_modules/@touchskyer/memex/skills/"
// install step.
package skills

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// All bundled SKILL.md files, addressed as embedded paths like
// "memex-recall/SKILL.md".
//
//go:embed all:memex-best-practices all:memex-organize all:memex-recall all:memex-retro all:memex-sync
var bundled embed.FS

// Action is what Install did for a particular skill.
type Action int

const (
	Installed Action = iota // file did not exist
	Updated                 // file existed with different content
	Unchanged               // file existed with identical content
)

func (a Action) String() string {
	switch a {
	case Installed:
		return "installed"
	case Updated:
		return "updated"
	case Unchanged:
		return "unchanged"
	}
	return "?"
}

// Result reports per-skill what Install did.
type Result struct {
	Skill  string
	Action Action
}

// Install writes every bundled SKILL.md to <targetDir>/<skill>/SKILL.md,
// creating parent directories as needed. Existing files are atomically
// overwritten only when their content differs. The order of returned results
// is deterministic (sorted by skill name).
func Install(targetDir string) ([]Result, error) {
	var results []Result
	err := fs.WalkDir(bundled, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, "/SKILL.md") {
			return nil
		}
		skillName := strings.TrimSuffix(path, "/SKILL.md")
		want, err := bundled.ReadFile(path)
		if err != nil {
			return err
		}
		dest := filepath.Join(targetDir, skillName, "SKILL.md")
		action, err := writeIfChanged(dest, want)
		if err != nil {
			return err
		}
		results = append(results, Result{Skill: skillName, Action: action})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// writeIfChanged is atomic-write semantics matching card.Store: tempfile
// + rename, only when content differs from existing. Returns the resulting
// Action.
func writeIfChanged(dest string, want []byte) (Action, error) {
	if existing, err := os.ReadFile(dest); err == nil {
		if bytes.Equal(existing, want) {
			return Unchanged, nil
		}
		if err := atomicWrite(dest, want); err != nil {
			return 0, err
		}
		return Updated, nil
	} else if !os.IsNotExist(err) {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return 0, err
	}
	if err := atomicWrite(dest, want); err != nil {
		return 0, err
	}
	return Installed, nil
}

func atomicWrite(dest string, data []byte) error {
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// Names returns the list of bundled skill names, sorted.
func Names() ([]string, error) {
	var out []string
	err := fs.WalkDir(bundled, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path != "." {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Format renders a Result line for CLI output.
func (r Result) Format() string {
	return fmt.Sprintf("  %-12s %s", r.Action, r.Skill)
}
