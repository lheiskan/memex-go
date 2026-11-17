package claudecode

import (
	"encoding/json"
	"os"
	"path/filepath"

	"memex/internal/orderedjson"
)

const Removed Action = "removed"

// Uninstall removes what Install wrote, and nothing else.
//
// "Nothing else" is the whole contract. Our own SessionStart group goes;
// foreign groups and every other settings member are re-emitted from their
// original bytes, and only the named skill directories are touched. A
// hand-written memex hook is left in place, as on install: removing a user's
// own config without asking is not this tool's call.
func Uninstall(skills fs, opts Options) ([]Step, []string, error) {
	dir, err := resolveDir(opts.ClaudeDir)
	if err != nil {
		return nil, nil, err
	}
	var steps []Step

	// --- skills.
	entries, err := skills.ReadDir("skills")
	if err != nil {
		return nil, nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		path := filepath.Join(dir, "skills", name, "SKILL.md")
		if _, serr := os.Stat(path); serr != nil {
			steps = append(steps, Step{Unchanged, name + " (absent)"})
			continue
		}
		if opts.DryRun {
			steps = append(steps, Step{WouldSet, name})
			continue
		}
		if rerr := os.Remove(path); rerr != nil {
			return nil, nil, rerr
		}
		// Prune the directory only if we emptied it; a user's own file in
		// there is theirs to keep.
		os.Remove(filepath.Dir(path))
		steps = append(steps, Step{Removed, name})
	}

	// --- hook.
	step, foreign, err := removeHook(dir, opts)
	if err != nil {
		return nil, nil, err
	}
	steps = append(steps, step)
	return steps, foreign, nil
}

func removeHook(dir string, opts Options) (Step, []string, error) {
	path := filepath.Join(dir, "settings.json")
	original, err := os.ReadFile(path)
	if err != nil {
		return Step{Unchanged, "no settings file"}, nil, nil
	}
	if !json.Valid(original) {
		return Step{}, nil, errNotJSON(path)
	}

	top, err := orderedjson.Decode(original)
	if err != nil {
		return Step{}, nil, err
	}
	events, groups, err := sessionStartGroups(top)
	if err != nil {
		return Step{}, nil, err
	}

	var foreign []string
	kept := make([]json.RawMessage, 0, len(groups))
	for _, g := range groups {
		if isOurs(g) {
			continue
		}
		if cmd, ok := foreignMemexCommand(g); ok {
			foreign = append(foreign, cmd)
		}
		kept = append(kept, g)
	}
	if len(kept) == len(groups) {
		return Step{Unchanged, "no hook of ours"}, foreign, nil
	}

	groupsRaw, err := encodeGroups(kept)
	if err != nil {
		return Step{}, nil, err
	}
	events = setPair(events, "SessionStart", groupsRaw)
	top = setPair(top, "hooks", orderedjson.Encode(events, 1))
	updated := append(orderedjson.Encode(top, 0), '\n')

	if opts.DryRun {
		return Step{WouldSet, "SessionStart hook"}, foreign, nil
	}
	if _, serr := os.Stat(path + backupSuffix); os.IsNotExist(serr) {
		if werr := writeFileAtomic(path+backupSuffix, original); werr != nil {
			return Step{}, nil, werr
		}
	}
	if werr := writeFileAtomic(path, updated); werr != nil {
		return Step{}, nil, werr
	}
	return Step{Removed, "SessionStart hook"}, foreign, nil
}
