package tikistore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A tiki that shows up while tiki runs gets no git lookup,
// so its file times are only a guess that a save must not write.
func TestSave_OmitsCreatedAtGuessedDuringLiveReload(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	writeDoc(t, filepath.Join(repo, "a.md"), "AAAAAA", "a")
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "-m", "a")
	s := newStoreIn(t, repo)

	b := filepath.Join(repo, "b.md")
	writeDoc(t, b, "BBBBBB", "b")
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "-m", "b", "--date", "2020-01-01T00:00:00Z")
	mustReload(t, s, b)
	retitleID(t, s, "BBBBBB")

	if strings.Contains(readFile(t, b), "createdAt") {
		t.Fatalf("a guessed createdAt was written:\n%s", readFile(t, b))
	}
	if err := s.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if want := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC); !s.GetTiki("BBBBBB").CreatedAt().Equal(want) {
		t.Errorf("after a full load CreatedAt = %v, want the commit's %v", s.GetTiki("BBBBBB").CreatedAt(), want)
	}
}

func TestSave_WritesFileTimesGitCannotContradict(t *testing.T) {
	for _, tc := range []struct {
		name string
		prep func(t *testing.T) (*TikiStore, string)
	}{
		{"live reload outside git", func(t *testing.T) (*TikiStore, string) {
			dir := t.TempDir()
			s := newStoreIn(t, dir)
			path := filepath.Join(dir, "a.md")
			writeDoc(t, path, "AAAAAA", "a")
			mustReload(t, s, path)
			return s, path
		}},
		{"full load, uncommitted in a repo", func(t *testing.T) (*TikiStore, string) {
			repo := t.TempDir()
			initRepo(t, repo)
			path := filepath.Join(repo, "a.md")
			writeDoc(t, path, "AAAAAA", "a")
			return newStoreIn(t, repo), path
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			forceBirthTime(t, forcedBirth, true)
			s, path := tc.prep(t)
			retitleID(t, s, "AAAAAA")
			if !strings.Contains(readFile(t, path), "createdAt: 2024-03-03T03:03:03Z") {
				t.Errorf("file time not written:\n%s", readFile(t, path))
			}
		})
	}
}

func TestReloadPaths_RenamedCommittedTikiSaveOmitsGuess(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	from := filepath.Join(repo, "a.md")
	writeDoc(t, from, "AAAAAA", "a")
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "-m", "a", "--date", "2020-01-01T00:00:00Z")
	s := newStoreIn(t, repo)

	to := filepath.Join(repo, "b.md")
	if err := os.Rename(from, to); err != nil {
		t.Fatal(err)
	}
	mustReload(t, s, from, to)
	retitleID(t, s, "AAAAAA")

	if strings.Contains(readFile(t, to), "createdAt") {
		t.Errorf("a guessed createdAt was written after the rename:\n%s", readFile(t, to))
	}
}

func TestSave_NestedRepoTikiOmitsGuess(t *testing.T) {
	dir := t.TempDir()
	inner := filepath.Join(dir, "inner")
	path := filepath.Join(inner, "a.md")
	writeDoc(t, path, "AAAAAA", "a")
	initRepo(t, inner)
	runGit(t, inner, "add", "-A")
	runGit(t, inner, "commit", "-q", "-m", "a")
	s := newStoreIn(t, dir)

	retitleID(t, s, "AAAAAA")

	if strings.Contains(readFile(t, path), "createdAt") {
		t.Errorf("a guessed createdAt was written into the nested repo's tiki:\n%s", readFile(t, path))
	}
}

// GIT_DIR without GIT_WORK_TREE makes git treat the working directory as the top level,
// so its paths no longer line up with tiki's.
func TestFullLoad_GitDirFromSubfolderStampsNothing(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	sub := filepath.Join(repo, "sub")
	path := filepath.Join(sub, "a.md")
	writeDoc(t, path, "AAAAAA", "a")
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "-m", "a")
	before := readFile(t, path)
	t.Setenv("GIT_DIR", filepath.Join(repo, ".git"))

	newStoreIn(t, sub, WithCreatedAtStamping())

	if got := readFile(t, path); got != before {
		t.Errorf("committed tiki stamped:\n%s", got)
	}
}

func TestStampCreatedAt_SkipsWriteLandingBeforeRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	writeDoc(t, path, "AAAAAA", "a")
	tk := newStoreIn(t, dir).GetTiki("AAAAAA")
	external := docContent("AAAAAA", "a longer title written by someone else")
	prev := readTikiFile
	readTikiFile = func(p string) ([]byte, error) {
		if err := os.WriteFile(p, []byte(external), 0o644); err != nil {
			return nil, err
		}
		return os.ReadFile(p)
	}
	t.Cleanup(func() { readTikiFile = prev })
	written := time.Now()

	if err := stampCreatedAt(tk); err == nil {
		t.Error("stampCreatedAt succeeded although the file changed before it was read")
	}
	if got := mustRead(t, path); got != external {
		t.Errorf("file =\n%s\nwant the outside write", got)
	}
	if got := mtimeOf(t, path); got.Before(written.Add(-time.Second)) {
		t.Errorf("the outside write's mtime was reset to %v", got)
	}
}

func TestStampCreatedAt_RecordsStateWhenRestoreFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	writeDoc(t, path, "AAAAAA", "a")
	s := newStoreIn(t, dir)
	tk := s.GetTiki("AAAAAA")
	prev := chtimes
	calls := 0
	chtimes = func(name string, atime, mtime time.Time) error {
		calls++
		if calls > 1 {
			return os.ErrPermission
		}
		return prev(name, atime, mtime)
	}
	t.Cleanup(func() { chtimes = prev })

	if err := stampCreatedAt(tk); err == nil {
		t.Error("stampCreatedAt reported success although the mtime was not restored")
	}
	chtimes = prev
	if !createdAtLine.MatchString(mustRead(t, path)) {
		t.Fatalf("file not stamped:\n%s", mustRead(t, path))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !tk.LoadedMtime.Equal(info.ModTime()) || tk.LoadedSize != info.Size() || !tk.CreatedAtStored() {
		t.Errorf("loaded state = (%v, %d, stored %v), file = (%v, %d)",
			tk.LoadedMtime, tk.LoadedSize, tk.CreatedAtStored(), info.ModTime(), info.Size())
	}
	retitleID(t, s, "AAAAAA")
}

func retitleID(t *testing.T, s *TikiStore, id string) {
	t.Helper()
	tk := s.GetTiki(id).Clone()
	tk.SetTitle("retitled")
	if err := s.UpdateTiki(tk); err != nil {
		t.Fatalf("UpdateTiki: %v", err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// relGit reports paths relative to root, optionally misspelling them as a buggy key builder would.
type relGit struct {
	countingGit
	root    string
	mangled bool
}

func (g *relGit) RepoRelPath(path string) (string, error) {
	rel, err := filepath.Rel(g.root, path)
	if err != nil {
		return "", err
	}
	if g.mangled && rel != "." {
		rel = "x-" + rel
	}
	return filepath.ToSlash(rel), nil
}

func TestHistoryKeysMatch(t *testing.T) {
	root := t.TempDir()
	tiki := filepath.Join(root, "a.md")
	writeDoc(t, tiki, "AAAAAA", "a")
	writeRaw(t, filepath.Join(root, "main.go"), "package main\n")
	walked := []string{tiki}
	at := time.Now()
	for _, tc := range []struct {
		name    string
		history map[string]time.Time
		mangled bool
		want    bool
	}{
		{"empty history", map[string]time.Time{}, false, true},
		{"a walked tiki has history", map[string]time.Time{"a.md": at}, false, true},
		{"history names only other files on disk", map[string]time.Time{"main.go": at}, false, true},
		{"history names nothing on disk", map[string]time.Time{"sub/a.md": at}, false, false},
		{"walked tiki under another key", map[string]time.Time{"a.md": at}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &TikiStore{dir: root, gitUtil: &relGit{root: root, mangled: tc.mangled}}
			if got := s.historyKeysMatch(tc.history, walked); got != tc.want {
				t.Errorf("historyKeysMatch = %v, want %v", got, tc.want)
			}
		})
	}
}
