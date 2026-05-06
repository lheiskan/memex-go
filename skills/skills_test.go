package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallToTempDir(t *testing.T) {
	dir := t.TempDir()
	results, err := Install(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"memex-best-practices",
		"memex-organize",
		"memex-recall",
		"memex-retro",
		"memex-sync",
	}
	if len(results) != len(want) {
		t.Fatalf("expected %d results, got %d", len(want), len(results))
	}
	for i, w := range want {
		if results[i].Skill != w {
			t.Errorf("result[%d]: got %s, want %s", i, results[i].Skill, w)
		}
		if results[i].Action != Installed {
			t.Errorf("result[%d] action: got %v, want Installed", i, results[i].Action)
		}
		path := filepath.Join(dir, w, "SKILL.md")
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("missing: %s (%v)", path, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("empty: %s", path)
		}
	}
}

func TestInstallIdempotent(t *testing.T) {
	dir := t.TempDir()
	if _, err := Install(dir); err != nil {
		t.Fatal(err)
	}
	results, err := Install(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.Action != Unchanged {
			t.Errorf("second install of %s: got %v, want Unchanged", r.Skill, r.Action)
		}
	}
}

func TestInstallDetectsUpdate(t *testing.T) {
	dir := t.TempDir()
	if _, err := Install(dir); err != nil {
		t.Fatal(err)
	}
	// Mutate one file, expect Updated on next install.
	target := filepath.Join(dir, "memex-recall", "SKILL.md")
	if err := os.WriteFile(target, []byte("stale content"), 0o644); err != nil {
		t.Fatal(err)
	}
	results, err := Install(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.Skill == "memex-recall" {
			if r.Action != Updated {
				t.Errorf("memex-recall: got %v, want Updated", r.Action)
			}
		} else if r.Action != Unchanged {
			t.Errorf("%s: got %v, want Unchanged", r.Skill, r.Action)
		}
	}
}

func TestNames(t *testing.T) {
	names, err := Names()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 5 {
		t.Errorf("got %d names, want 5: %v", len(names), names)
	}
}
