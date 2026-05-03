package card

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateSlug(t *testing.T) {
	good := []string{"foo", "foo-bar", "adr-001", "nested/path"}
	for _, s := range good {
		if err := ValidateSlug(s); err != nil {
			t.Errorf("ValidateSlug(%q): unexpected error %v", s, err)
		}
	}
	bad := []string{"", "  ", ".", "..", "a/../b", "/foo", "foo/", "a//b", "foo:bar", "foo*bar"}
	for _, s := range bad {
		if err := ValidateSlug(s); err == nil {
			t.Errorf("ValidateSlug(%q): expected error, got nil", s)
		}
	}
}

func TestStoreWriteReadArchive(t *testing.T) {
	dir := t.TempDir()
	cardsDir := filepath.Join(dir, "cards")
	archiveDir := filepath.Join(dir, "archive")
	_ = os.MkdirAll(cardsDir, 0o755)
	_ = os.MkdirAll(archiveDir, 0o755)
	s := NewStore(cardsDir, archiveDir, false)

	content := []byte("---\ntitle: Hello\n---\nbody\n")
	if err := s.WriteCard("hello", content); err != nil {
		t.Fatalf("WriteCard: %v", err)
	}
	got, err := s.ReadCard("hello")
	if err != nil {
		t.Fatalf("ReadCard: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("read mismatch: got %q, want %q", got, content)
	}
	cards, err := s.ScanAll()
	if err != nil {
		t.Fatalf("ScanAll: %v", err)
	}
	if len(cards) != 1 || cards[0].Slug != "hello" {
		t.Errorf("ScanAll: got %v", cards)
	}
	if err := s.ArchiveCard("hello"); err != nil {
		t.Fatalf("ArchiveCard: %v", err)
	}
	if _, err := s.ReadCard("hello"); err == nil {
		t.Errorf("expected ReadCard to fail after archive")
	}
	if _, err := os.Stat(filepath.Join(archiveDir, "hello.md")); err != nil {
		t.Errorf("archive file missing: %v", err)
	}
}

func TestStoreRejectsTraversalSlug(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "cards"), filepath.Join(dir, "archive"), false)
	_ = os.MkdirAll(s.CardsDir, 0o755)
	if err := s.WriteCard("../escape", []byte("---\n---\n")); err == nil {
		t.Error("expected slug validation to reject traversal")
	}
}

func TestNoTempLeftAfterCrash(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "cards"), filepath.Join(dir, "archive"), false)
	_ = os.MkdirAll(s.CardsDir, 0o755)
	if err := s.WriteCard("foo", []byte("x")); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(s.CardsDir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("temp file leaked: %s", e.Name())
		}
	}
}
