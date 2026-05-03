package card

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// reservedChars are not allowed in slugs (OS-reserved or dangerous).
var reservedChars = regexp.MustCompile(`[:*?"<>|]`)

// ValidateSlug returns nil if the slug is acceptable for writing, or an error
// describing what's wrong. Mirrors lib/store.js validateSlug exactly.
func ValidateSlug(slug string) error {
	trimmed := strings.TrimSpace(slug)
	if trimmed == "" {
		return fmt.Errorf("invalid slug: must not be empty or whitespace-only")
	}
	if onlyDotsAndSlashes(trimmed) {
		return fmt.Errorf("invalid slug: must not consist only of dots and slashes")
	}
	if reservedChars.MatchString(trimmed) {
		return fmt.Errorf(`invalid slug: contains reserved characters (: * ? " < > |)`)
	}
	if strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, `\`) ||
		strings.HasSuffix(trimmed, "/") || strings.HasSuffix(trimmed, `\`) ||
		strings.Contains(trimmed, "//") || strings.Contains(trimmed, `\\`) {
		return fmt.Errorf("invalid slug: must not contain empty path segments")
	}
	for _, seg := range regexp.MustCompile(`[/\\]`).Split(trimmed, -1) {
		if seg == "." || seg == ".." {
			return fmt.Errorf("invalid slug: path segments must not be '.' or '..'")
		}
	}
	return nil
}

func onlyDotsAndSlashes(s string) bool {
	for _, r := range s {
		if r != '.' && r != '/' && r != '\\' {
			return false
		}
	}
	return true
}

// Store wraps a cards directory and an archive directory, both expected to
// live under a memex home (typically ~/.memex/). Methods are safe for
// concurrent reads but writes mutate the in-memory scan cache and should not
// race with each other. The CLI/MCP server is single-threaded enough that we
// don't need finer-grained locking yet.
type Store struct {
	CardsDir    string
	ArchiveDir  string
	NestedSlugs bool

	mu        sync.Mutex
	scanCache []ScanEntry
}

type ScanEntry struct {
	Slug string
	Path string
}

func NewStore(cardsDir, archiveDir string, nestedSlugs bool) *Store {
	return &Store{CardsDir: cardsDir, ArchiveDir: archiveDir, NestedSlugs: nestedSlugs}
}

// Home returns the parent directory of CardsDir — typically ~/.memex.
// Used by callers that need to find sibling files (.sync.json, etc.).
func (s *Store) Home() string {
	return filepath.Dir(s.CardsDir)
}

func (s *Store) invalidateCache() {
	s.mu.Lock()
	s.scanCache = nil
	s.mu.Unlock()
}

// ScanAll walks CardsDir recursively and returns every .md file. Result is
// memoized until the next write. Slug computation depends on NestedSlugs:
// off (default) → basename without extension; on → relative path with /
// separators, also without extension.
func (s *Store) ScanAll() ([]ScanEntry, error) {
	s.mu.Lock()
	if s.scanCache != nil {
		out := s.scanCache
		s.mu.Unlock()
		return out, nil
	}
	s.mu.Unlock()

	var results []ScanEntry
	err := filepath.Walk(s.CardsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}
		var slug string
		if s.NestedSlugs {
			rel, err := filepath.Rel(s.CardsDir, path)
			if err != nil {
				return err
			}
			rel = strings.TrimSuffix(rel, ".md")
			slug = filepath.ToSlash(rel)
		} else {
			slug = strings.TrimSuffix(info.Name(), ".md")
		}
		results = append(results, ScanEntry{Slug: slug, Path: path})
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.scanCache = results
	s.mu.Unlock()
	return results, nil
}

// Resolve returns the file path for a slug, or "" if not found.
func (s *Store) Resolve(slug string) (string, error) {
	cards, err := s.ScanAll()
	if err != nil {
		return "", err
	}
	normalised := strings.ReplaceAll(slug, `\`, "/")
	for _, c := range cards {
		if c.Slug == normalised {
			return c.Path, nil
		}
	}
	return "", nil
}

// ReadCard returns the full file contents for a slug.
func (s *Store) ReadCard(slug string) ([]byte, error) {
	path, err := s.Resolve(slug)
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, fmt.Errorf("card not found: %s", slug)
	}
	return os.ReadFile(path)
}

// WriteCard writes content to the slug's file path using a temp+rename
// atomic write. If the slug doesn't exist yet, it is created at
// CardsDir/<slug>.md. Asserts that the resolved path stays within CardsDir.
func (s *Store) WriteCard(slug string, content []byte) error {
	if err := ValidateSlug(slug); err != nil {
		return err
	}
	existing, err := s.Resolve(slug)
	if err != nil {
		return err
	}
	target := existing
	if target == "" {
		target = filepath.Join(s.CardsDir, slug+".md")
	}
	if err := assertSafePath(target, s.CardsDir); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	s.invalidateCache()
	return nil
}

// ArchiveCard moves a card from CardsDir to ArchiveDir, preserving any nested
// path. Returns an error if the card doesn't exist in CardsDir; if it already
// exists in ArchiveDir, returns "already archived".
func (s *Store) ArchiveCard(slug string) error {
	if err := ValidateSlug(slug); err != nil {
		return err
	}
	path, err := s.Resolve(slug)
	if err != nil {
		return err
	}
	if path == "" {
		// Check if already archived.
		archivePath := filepath.Join(s.ArchiveDir, slug+".md")
		if _, statErr := os.Stat(archivePath); statErr == nil {
			return fmt.Errorf("card already archived: %s", slug)
		}
		return fmt.Errorf("card not found: %s", slug)
	}
	dest := filepath.Join(s.ArchiveDir, slug+".md")
	if err := assertSafePath(dest, s.ArchiveDir); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := os.Rename(path, dest); err != nil {
		return err
	}
	s.invalidateCache()
	return nil
}

func assertSafePath(target, root string) error {
	resolved, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	rootResolved, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(resolved, rootResolved+string(filepath.Separator)) && resolved != rootResolved {
		return fmt.Errorf("invalid slug: path escapes %s", root)
	}
	return nil
}
