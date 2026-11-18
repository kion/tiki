package tikistore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gitops "github.com/boolean-maybe/tiki/store/internal/git"
)

const commitDateLine = "createdAt: 2020-01-01T00:00:00Z"

// alice commits while the repo's configured user stays testuser,
// so an author taken from git is unmistakable.
func repoWithCommittedTiki(t *testing.T, rel string) (repo, path string) {
	t.Helper()
	repo = t.TempDir()
	initRepo(t, repo)
	writeDoc(t, filepath.Join(repo, "keep.md"), "KKKKKK", "keep")
	path = filepath.Join(repo, filepath.FromSlash(rel))
	writeDoc(t, path, "AAAAAA", "a")
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "-c", "user.name=alice", "-c", "user.email=alice@example.com",
		"commit", "-q", "-m", "add", "--date", "2020-01-01T00:00:00Z")
	return repo, path
}

func editBody(t *testing.T, path string) {
	t.Helper()
	writeRaw(t, path, strings.Replace(readFile(t, path), "body\n", "body edited\n", 1))
}

func TestFullLoad_StampsChangedCommittedTiki(t *testing.T) {
	for _, tc := range []struct {
		name string
		prep func(t *testing.T, repo, path string)
	}{
		{"modified", func(t *testing.T, repo, path string) { editBody(t, path) }},
		{"staged", func(t *testing.T, repo, path string) {
			editBody(t, path)
			runGit(t, repo, "add", "a.md")
		}},
		{"staged and modified again", func(t *testing.T, repo, path string) {
			editBody(t, path)
			runGit(t, repo, "add", "a.md")
			writeRaw(t, path, readFile(t, path)+"more\n")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, path := repoWithCommittedTiki(t, "a.md")
			tc.prep(t, repo, path)
			april := time.Date(2024, 4, 4, 0, 0, 0, 0, time.UTC)
			setMtime(t, path, april)

			newStoreIn(t, repo, WithCreatedAtStamping())

			if got := strings.Count(readFile(t, path), commitDateLine); got != 1 {
				t.Errorf("file has %d commit-date lines, want 1:\n%s", got, readFile(t, path))
			}
			if got := mtimeOf(t, path); !got.Equal(april) {
				t.Errorf("mtime = %v, want it kept at %v", got, april)
			}
		})
	}
}

func TestFullLoad_RecognisesMovedTiki(t *testing.T) {
	for _, tc := range []struct {
		name     string
		from, to string
		move     func(t *testing.T, repo, from, to string)
	}{
		{"mv into a folder", "a.md", "done/a.md", plainMove},
		{"git mv", "a.md", "b.md", func(t *testing.T, repo, from, to string) {
			runGit(t, repo, "mv", from, to)
		}},
		{"mv and edit", "a.md", "done/a.md", func(t *testing.T, repo, from, to string) {
			plainMove(t, repo, from, to)
			editBody(t, filepath.Join(repo, to))
		}},
		{"mv from a non-ASCII path", "Ü/a.md", "a.md", plainMove},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, _ := repoWithCommittedTiki(t, tc.from)
			tc.move(t, repo, tc.from, tc.to)

			s := newStoreIn(t, repo, WithCreatedAtStamping())

			if got := readFile(t, filepath.Join(repo, tc.to)); !strings.Contains(got, commitDateLine) {
				t.Errorf("moved tiki not stamped with its commit date:\n%s", got)
			}
			if v, _ := s.GetTiki("AAAAAA").Get("createdBy"); v != "alice" {
				t.Errorf("createdBy = %v, want the original author alice", v)
			}
		})
	}
}

func plainMove(t *testing.T, repo, from, to string) {
	t.Helper()
	dst := filepath.Join(repo, filepath.FromSlash(to))
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(repo, filepath.FromSlash(from)), dst); err != nil {
		t.Fatal(err)
	}
}

func TestFullLoad_MoveIntoNestedRepoNotStamped(t *testing.T) {
	repo, _ := repoWithCommittedTiki(t, "a.md")
	inner := filepath.Join(repo, "inner")
	if err := os.Mkdir(inner, 0o750); err != nil {
		t.Fatal(err)
	}
	initRepo(t, inner)
	plainMove(t, repo, "a.md", "inner/a.md")

	newStoreIn(t, repo, WithCreatedAtStamping())

	if got := readFile(t, filepath.Join(inner, "a.md")); strings.Contains(got, "createdAt") {
		t.Errorf("tiki moved into a nested repo was stamped:\n%s", got)
	}
}

func TestFullLoad_MovedTikiValueWithoutStamping(t *testing.T) {
	repo, _ := repoWithCommittedTiki(t, "a.md")
	plainMove(t, repo, "a.md", "done/a.md")

	s := newStoreIn(t, repo)
	if want := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC); !s.GetTiki("AAAAAA").CreatedAt().Equal(want) {
		t.Errorf("CreatedAt = %v, want the commit's %v", s.GetTiki("AAAAAA").CreatedAt(), want)
	}
	retitleID(t, s, "AAAAAA")

	if got := readFile(t, filepath.Join(repo, "done", "a.md")); !strings.Contains(got, commitDateLine) {
		t.Errorf("a save didn't write the commit date:\n%s", got)
	}
}

// movesGit scripts what git reports, around a committed keep.md that stays on disk,
// so the history-key guard keeps the history.
type movesGit struct {
	relGit
	authors    map[string]*gitops.AuthorInfo
	changes    map[string]gitops.FileChange
	heads      map[string][]byte
	changesErr error
	headsErr   error
}

func (g *movesGit) AllAuthors(string) (map[string]*gitops.AuthorInfo, error) { return g.authors, nil }
func (g *movesGit) AllLastCommitTimes(string) (map[string]time.Time, error) {
	return map[string]time.Time{"keep.md": time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}, nil
}
func (g *movesGit) ChangedFiles(string) (map[string]gitops.FileChange, error) {
	return g.changes, g.changesErr
}
func (g *movesGit) HeadContents(paths []string) (map[string][]byte, error) {
	if g.headsErr != nil {
		return nil, g.headsErr
	}
	out := map[string][]byte{}
	for _, p := range paths {
		if c, ok := g.heads[p]; ok {
			out[p] = c
		}
	}
	return out, nil
}

func storeWithScriptedGit(t *testing.T, g *movesGit) (*TikiStore, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeDoc(t, filepath.Join(dir, "keep.md"), "KKKKKK", "keep")
	path := filepath.Join(dir, "new.md")
	writeDoc(t, path, "NNNNNN", "n")
	s := newStoreIn(t, dir, WithCreatedAtStamping())
	g.root = dir
	s.gitUtil = g
	if err := s.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	return s, path
}

func headWithID(id string) []byte {
	return []byte("---\nid: " + id + "\ntitle: old\n---\nbody\n")
}

func assertGuessKept(t *testing.T, s *TikiStore, path string) {
	t.Helper()
	if strings.Contains(readFile(t, path), "createdAt") {
		t.Fatalf("stamped:\n%s", readFile(t, path))
	}
	retitleID(t, s, "NNNNNN")
	if strings.Contains(readFile(t, path), "createdAt") {
		t.Errorf("a save wrote a guessed createdAt:\n%s", readFile(t, path))
	}
}

func TestFullLoad_MovedTikiWithoutAddCommitStaysGuess(t *testing.T) {
	s, path := storeWithScriptedGit(t, &movesGit{
		changes: map[string]gitops.FileChange{"old.md": gitops.FileGone},
		heads:   map[string][]byte{"old.md": headWithID("NNNNNN")},
	})
	assertGuessKept(t, s, path)
}

func TestFullLoad_AmbiguousMovedIDStaysGuess(t *testing.T) {
	s, path := storeWithScriptedGit(t, &movesGit{
		changes: map[string]gitops.FileChange{"old1.md": gitops.FileGone, "old2.md": gitops.FileGone},
		heads:   map[string][]byte{"old1.md": headWithID("NNNNNN"), "old2.md": headWithID("NNNNNN")},
		authors: map[string]*gitops.AuthorInfo{
			"old1.md": {Name: "alice", Date: time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)},
			"old2.md": {Name: "bob", Date: time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
	})
	assertGuessKept(t, s, path)
}

func TestFullLoad_GitStatusFailureTrustsNoNewFile(t *testing.T) {
	for _, tc := range []struct {
		name string
		git  *movesGit
	}{
		{"ChangedFiles fails", &movesGit{changesErr: os.ErrPermission}},
		{"HeadContents fails", &movesGit{
			changes:  map[string]gitops.FileChange{"old.md": gitops.FileGone},
			headsErr: os.ErrPermission,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, path := storeWithScriptedGit(t, tc.git)
			assertGuessKept(t, s, path)
		})
	}
}

func TestFullLoad_ScriptedGitControlStampsNewFile(t *testing.T) {
	_, path := storeWithScriptedGit(t, &movesGit{
		changes: map[string]gitops.FileChange{"old.md": gitops.FileGone},
		heads:   map[string][]byte{"old.md": headWithID("OOOOOO")},
	})
	if !createdAtLine.MatchString(readFile(t, path)) {
		t.Errorf("control: a genuinely new tiki was not stamped:\n%s", readFile(t, path))
	}
}

func commitAsAlice(t *testing.T, repo string) {
	t.Helper()
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "-c", "user.name=alice", "-c", "user.email=alice@example.com",
		"commit", "-q", "-m", "add", "--date", "2020-01-01T00:00:00Z")
}

func withoutCreatedAtLine(content string) string {
	return strings.Replace(content, "createdAt: 2019-06-01T00:00:00Z\n", "", 1)
}

// HEAD's stored createdAt is older information than the add commit,
// so a lost line comes back with it.
func TestFullLoad_ChangedTikiTakesCreatedAtFromHead(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	writeDoc(t, filepath.Join(repo, "keep.md"), "KKKKKK", "keep")
	path := filepath.Join(repo, "a.md")
	writeRaw(t, path, docWithCreatedLine("a", "createdAt: 2019-06-01T00:00:00Z"))
	commitAsAlice(t, repo)
	writeRaw(t, path, strings.Replace(withoutCreatedAtLine(readFile(t, path)), "body\n", "body edited\n", 1))

	newStoreIn(t, repo, WithCreatedAtStamping())

	if got := readFile(t, path); !strings.Contains(got, "createdAt: 2019-06-01T00:00:00Z") {
		t.Errorf("the value at HEAD wasn't restored:\n%s", got)
	}
}

func TestFullLoad_MovedTikiTakesCreatedAtFromHead(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	writeDoc(t, filepath.Join(repo, "keep.md"), "KKKKKK", "keep")
	writeRaw(t, filepath.Join(repo, "a.md"), docWithCreatedLine("a", "createdAt: 2019-06-01T00:00:00Z"))
	commitAsAlice(t, repo)
	plainMove(t, repo, "a.md", "done/a.md")
	moved := filepath.Join(repo, "done", "a.md")
	writeRaw(t, moved, withoutCreatedAtLine(readFile(t, moved)))

	newStoreIn(t, repo, WithCreatedAtStamping())

	if got := readFile(t, moved); !strings.Contains(got, "createdAt: 2019-06-01T00:00:00Z") {
		t.Errorf("the moved tiki didn't get the value at HEAD:\n%s", got)
	}
}

// A new tiki written where a moved one used to be inherits nothing from that path's history.
func TestFullLoad_ReusedPathIsNotStampedWithOldDate(t *testing.T) {
	repo, _ := repoWithCommittedTiki(t, "a.md")
	plainMove(t, repo, "a.md", "done/a.md")
	reused := filepath.Join(repo, "a.md")
	writeDoc(t, reused, "BBBBBB", "b")

	s := newStoreIn(t, repo, WithCreatedAtStamping())

	if got := readFile(t, reused); strings.Contains(got, "createdAt") {
		t.Errorf("the new tiki at a reused path was stamped:\n%s", got)
	}
	if v, _ := s.GetTiki("BBBBBB").Get("createdBy"); v == "alice" {
		t.Error("the new tiki took the old tiki's author")
	}
	if got := readFile(t, filepath.Join(repo, "done", "a.md")); !strings.Contains(got, commitDateLine) {
		t.Errorf("the moved tiki wasn't recognised:\n%s", got)
	}
}

func TestFullLoad_ChangedTikiStampedOnlyWhenHeadConfirmsIt(t *testing.T) {
	for _, tc := range []struct {
		name      string
		heads     map[string][]byte
		headsErr  error
		wantStamp bool
	}{
		{"HEAD unreadable", nil, os.ErrPermission, false},
		{"HEAD holds the same tiki", map[string][]byte{"keep.md": headWithID("KKKKKK")}, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := storeWithScriptedGit(t, &movesGit{
				authors:  map[string]*gitops.AuthorInfo{"keep.md": {Name: "alice", Date: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}},
				changes:  map[string]gitops.FileChange{"keep.md": gitops.FileChanged},
				heads:    tc.heads,
				headsErr: tc.headsErr,
			})
			keep := filepath.Join(s.dir, "keep.md")
			if got := strings.Contains(readFile(t, keep), commitDateLine); got != tc.wantStamp {
				t.Errorf("stamped = %v, want %v:\n%s", got, tc.wantStamp, readFile(t, keep))
			}
		})
	}
}

// countingWorkingTree counts the git calls only a creation-time decision needs.
type countingWorkingTree struct {
	movesGit
	calls int
}

func (g *countingWorkingTree) ChangedFiles(dir string) (map[string]gitops.FileChange, error) {
	g.calls++
	return g.movesGit.ChangedFiles(dir)
}

func (g *countingWorkingTree) HeadContents(paths []string) (map[string][]byte, error) {
	g.calls++
	return g.movesGit.HeadContents(paths)
}

func TestFullLoad_AsksGitStatusOnlyWhenACreationTimeDependsOnIt(t *testing.T) {
	for _, tc := range []struct {
		name      string
		newTiki   bool
		wantCalls bool
	}{
		{"every tiki has a stored createdAt", false, false},
		{"a tiki without one", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, ".git"), 0o750); err != nil {
				t.Fatal(err)
			}
			writeRaw(t, filepath.Join(dir, "keep.md"),
				"---\nid: KKKKKK\ntitle: keep\ncreatedAt: 2020-01-01T00:00:00Z\n---\nbody\n")
			if tc.newTiki {
				writeDoc(t, filepath.Join(dir, "new.md"), "NNNNNN", "n")
			}
			s := newStoreIn(t, dir, WithCreatedAtStamping())
			g := &countingWorkingTree{movesGit: movesGit{relGit: relGit{root: dir}}}
			s.gitUtil = g
			if err := s.Reload(); err != nil {
				t.Fatalf("Reload: %v", err)
			}
			if got := g.calls > 0; got != tc.wantCalls {
				t.Errorf("working-tree git calls = %d, want any: %v", g.calls, tc.wantCalls)
			}
		})
	}
}
