package document

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// ignoreFileNames are the root-level files whose patterns decide which paths
// count as documents. A change to either one changes the scope itself.
var ignoreFileNames = []string{".gitignore", ".tikiignore"}

// Scope answers "is this path a managed document (or a directory that may
// hold one)?" for a single scan root. It is the shared rule set behind
// WalkDocuments, the store's incremental reload and the live-reload watcher,
// so all three agree on exactly which files are documents.
//
// Rules (see WalkDocuments): `.md` files only (case-insensitive); hidden
// directories are pruned except `.doc`; paths matched by the root
// `.gitignore`/`.tikiignore` are excluded. Hidden *file* names are allowed.
//
// The ignore patterns are read once by NewScope; build a new Scope after an
// ignore file changes.
type Scope struct {
	root    string // as given (cleaned); Walk reports paths under this form
	absRoot string // absolute form, used to classify paths given in any form
	ignore  *IgnoreMatcher
}

// NewScope loads the ignore patterns at root and returns its scope.
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

// Root returns the scan root.
func (s *Scope) Root() string { return s.root }

// IncludeDir reports whether path is the root or a directory beneath it that
// the document walk descends into. Every ancestor between the root and path
// must pass too. The directory need not exist.
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

// IncludeFile reports whether path is a managed document location: a `.md`
// file under an included directory that no ignore pattern matches. The file
// need not exist, so removed and renamed paths classify the same way.
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

// IsIgnoreFile reports whether path is one of the root-level ignore files.
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

// Walk walks start (which must lie inside the scope) and calls fn for every
// in-scope directory, start included, and every in-scope document beneath it,
// pruning out-of-scope subtrees. An out-of-scope start visits nothing.
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
			// start's ancestors were checked by IncludeDir above; every other
			// directory's ancestors passed on the way down, so checking the
			// entry itself is enough.
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

// subdirIncluded checks one directory below the root on its own (rel is its
// root-relative path), assuming its ancestors are already included. It
// applies IsSkippableSubdir's hidden-name rule and the ignore patterns.
func (s *Scope) subdirIncluded(rel, name string) bool {
	if isSkippableDirName(name) {
		return false
	}
	return !s.ignore.Match(rel, true)
}

// rel returns path relative to the root, or false when path lies outside it.
// Both sides are compared in absolute form so a relative root and absolute
// event paths (or vice versa) classify the same way.
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
