package document

import (
	"io/fs"
	"path/filepath"
	"strings"
)

var ignoreFileNames = []string{".gitignore", ".tikiignore"}

// Scope is shared by the walker, the store's incremental reload and the live-reload watcher,
// so all three agree on which files are documents.
// Ignore patterns are read once:
// build a new Scope when an ignore file changes.
type Scope struct {
	root    string // Walk reports paths in the form the caller used
	absRoot string // classification accepts paths in any form
	ignore  *IgnoreMatcher
}

func NewScope(root string) (*Scope, error) {
	ignore, err := LoadIgnoreMatcher(root)
	if err != nil {
		return nil, err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Scope{root: filepath.Clean(root), absRoot: absRoot, ignore: ignore}, nil
}

func (s *Scope) Root() string { return s.root }

func (s *Scope) IncludeDir(path string) bool {
	rel, ok := s.rel(path)
	if !ok {
		return false
	}
	if rel == "." {
		return true
	}
	parts := strings.Split(rel, string(filepath.Separator))
	for i, name := range parts {
		if !s.subdirIncluded(filepath.Join(parts[:i+1]...), name) {
			return false
		}
	}
	return true
}

// The file need not exist, so removed and renamed paths classify too.
func (s *Scope) IncludeFile(path string) bool {
	if !strings.EqualFold(filepath.Ext(path), ".md") {
		return false
	}
	rel, ok := s.rel(path)
	if !ok || rel == "." {
		return false
	}
	if !s.IncludeDir(filepath.Dir(filepath.Clean(path))) {
		return false
	}
	return !s.ignore.Match(rel, false)
}

func (s *Scope) IsIgnoreFile(path string) bool {
	rel, ok := s.rel(path)
	if !ok || strings.ContainsRune(rel, filepath.Separator) {
		return false
	}
	base := rel
	for _, name := range ignoreFileNames {
		if base == name {
			return true
		}
	}
	return false
}

func (s *Scope) Walk(start string, fn func(path string, isDir bool) error) error {
	start = filepath.Clean(start)
	if !s.IncludeDir(start) {
		return nil
	}
	return filepath.WalkDir(start, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, ok := s.rel(path)
		if !ok {
			return nil
		}
		if d.IsDir() {
			// ancestors passed on the way down (start's via IncludeDir)
			if path != start && !s.subdirIncluded(rel, d.Name()) {
				return filepath.SkipDir
			}
			return fn(path, true)
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".md") {
			return nil
		}
		if s.ignore.Match(rel, false) {
			return nil
		}
		return fn(path, false)
	})
}

// Assumes the ancestors are already included.
func (s *Scope) subdirIncluded(rel, name string) bool {
	if isSkippableDirName(name) {
		return false
	}
	return !s.ignore.Match(rel, true)
}

// Absolute on both sides:
// the root may be relative while event paths are absolute.
func (s *Scope) rel(path string) (string, bool) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(s.absRoot, absPath)
	if err != nil {
		return "", false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}
