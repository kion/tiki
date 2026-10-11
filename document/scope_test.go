package document

import (
	"io/fs"
	"path/filepath"
	"sort"
	"testing"
)

func scopeFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, ".gitignore"), "node_modules/\nbuild/\ndrafts/*\n!drafts/keep.md\n")
	mustWrite(t, filepath.Join(root, ".tikiignore"), "DRAFT0.md\n")

	for _, rel := range []string{
		"AAAAAA.md",
		"UPPER.MD",
		".notes.md",
		"DRAFT0.md",
		"readme.txt",
		"projects/BBBBBB.md",
		"projects/deep/CCCCCC.md",
		"projects/build/ZZZZZZ.md",
		"node_modules/pkg/ZZZZZZ.md",
		".git/ZZZZZZ.md",
		".obsidian/cache/ZZZZZZ.md",
		".doc/DDDDDD.md",
		".doc/tiki/EEEEEE.md",
		"sub/.doc/FFFFFF.md",
		"sub/.hidden/ZZZZZZ.md",
		"drafts/skip.md",
		"drafts/keep.md",
	} {
		mustWrite(t, filepath.Join(root, filepath.FromSlash(rel)), "x\n")
	}
	return root
}

func mustScope(t *testing.T, root string) *Scope {
	t.Helper()
	s, err := NewScope(root)
	if err != nil {
		t.Fatalf("NewScope: %v", err)
	}
	return s
}

func TestScope_IncludeFile(t *testing.T) {
	root := scopeFixture(t)
	s := mustScope(t, root)

	cases := map[string]bool{
		"AAAAAA.md":                  true,
		"UPPER.MD":                   true,
		".notes.md":                  true,
		"DRAFT0.md":                  false,
		"readme.txt":                 false,
		"projects/BBBBBB.md":         true,
		"projects/deep/CCCCCC.md":    true,
		"projects/build/ZZZZZZ.md":   false,
		"node_modules/pkg/ZZZZZZ.md": false,
		".git/ZZZZZZ.md":             false,
		".obsidian/cache/ZZZZZZ.md":  false,
		".doc/DDDDDD.md":             true,
		".doc/tiki/EEEEEE.md":        true,
		"sub/.doc/FFFFFF.md":         true,
		"sub/.hidden/ZZZZZZ.md":      false,
		"drafts/skip.md":             false,
		"drafts/keep.md":             true,
		"projects/NEWNEW.md":         true, // nonexistent: removed paths must classify too
	}
	for rel, want := range cases {
		if got := s.IncludeFile(filepath.Join(root, filepath.FromSlash(rel))); got != want {
			t.Errorf("IncludeFile(%s) = %v, want %v", rel, got, want)
		}
	}

	if s.IncludeFile(filepath.Join(filepath.Dir(root), "outside.md")) {
		t.Error("IncludeFile accepted a path outside the root")
	}
	if s.IncludeFile(root) {
		t.Error("IncludeFile accepted the root directory itself")
	}
}

func TestScope_IncludeDir(t *testing.T) {
	root := scopeFixture(t)
	s := mustScope(t, root)

	cases := map[string]bool{
		".":                true,
		"projects":         true,
		"projects/deep":    true,
		"projects/build":   false,
		"node_modules":     false,
		"node_modules/pkg": false,
		".git":             false,
		".obsidian/cache":  false,
		".doc":             true,
		".doc/tiki":        true,
		"sub/.doc":         true,
		"sub/.hidden":      false,
		"drafts":           true,
	}
	for rel, want := range cases {
		if got := s.IncludeDir(filepath.Join(root, filepath.FromSlash(rel))); got != want {
			t.Errorf("IncludeDir(%s) = %v, want %v", rel, got, want)
		}
	}
	if s.IncludeDir(filepath.Dir(root)) {
		t.Error("IncludeDir accepted the root's parent")
	}
}

func TestScope_IsIgnoreFile(t *testing.T) {
	root := t.TempDir()
	s := mustScope(t, root)

	cases := map[string]bool{
		".gitignore":      true,
		".tikiignore":     true,
		"sub/.gitignore":  false,
		"sub/.tikiignore": false,
		"other":           false,
	}
	for rel, want := range cases {
		if got := s.IsIgnoreFile(filepath.Join(root, filepath.FromSlash(rel))); got != want {
			t.Errorf("IsIgnoreFile(%s) = %v, want %v", rel, got, want)
		}
	}
}

func TestScope_ParityWithWalkDocuments(t *testing.T) {
	root := scopeFixture(t)
	s := mustScope(t, root)

	var classified []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && s.IncludeFile(path) {
			classified = append(classified, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("raw walk: %v", err)
	}
	sort.Strings(classified)

	walked, err := WalkDocuments(root)
	if err != nil {
		t.Fatalf("WalkDocuments: %v", err)
	}
	if len(walked) != len(classified) {
		t.Fatalf("parity mismatch:\n walk:     %v\n classify: %v", walked, classified)
	}
	for i := range walked {
		if walked[i] != classified[i] {
			t.Fatalf("parity mismatch at %d:\n walk:     %v\n classify: %v", i, walked, classified)
		}
	}
}

func TestScope_WalkFromSubdir(t *testing.T) {
	root := scopeFixture(t)
	s := mustScope(t, root)

	var dirs, files []string
	err := s.Walk(filepath.Join(root, "projects"), func(path string, isDir bool) error {
		rel, _ := filepath.Rel(root, path)
		if isDir {
			dirs = append(dirs, filepath.ToSlash(rel))
		} else {
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	sort.Strings(dirs)
	sort.Strings(files)

	wantDirs := []string{"projects", "projects/deep"}
	wantFiles := []string{"projects/BBBBBB.md", "projects/deep/CCCCCC.md"}
	if !equalStrings(dirs, wantDirs) {
		t.Errorf("dirs = %v, want %v", dirs, wantDirs)
	}
	if !equalStrings(files, wantFiles) {
		t.Errorf("files = %v, want %v", files, wantFiles)
	}
}

func TestScope_WalkOutOfScopeStartIsNoop(t *testing.T) {
	root := scopeFixture(t)
	s := mustScope(t, root)

	called := false
	if err := s.Walk(filepath.Join(root, "node_modules"), func(string, bool) error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if called {
		t.Error("Walk visited an out-of-scope start directory")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
