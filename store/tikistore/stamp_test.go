package tikistore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gitops "github.com/boolean-maybe/tiki/store/internal/git"
)

type failingHistoryGit struct{ countingGit }

func (g *failingHistoryGit) AllAuthors(string) (map[string]*gitops.AuthorInfo, error) {
	return nil, errors.New("history unreadable")
}

func (g *failingHistoryGit) AllLastCommitTimes(string) (map[string]time.Time, error) {
	return nil, errors.New("history unreadable")
}

func initRepo(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "testuser")
}

func setMtime(t *testing.T, path string, at time.Time) {
	t.Helper()
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
}

func mtimeOf(t *testing.T, path string) time.Time {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	return info.ModTime()
}

func TestInsertCreatedAtLine(t *testing.T) {
	created := time.Date(2024, 3, 3, 5, 3, 3, 900_000_000, time.FixedZone("", 2*3600))
	for _, tc := range []struct {
		name, in, want string
		ok             bool
	}{
		{"after title", "---\nid: AAAAAA\ntitle: a\n---\nbody\n",
			"---\nid: AAAAAA\ntitle: a\ncreatedAt: 2024-03-03T03:03:03Z\n---\nbody\n", true},
		{"CRLF", "---\r\nid: AAAAAA\r\ntitle: a\r\nstatus: x\r\n---\r\nbody\r\n",
			"---\r\nid: AAAAAA\r\ntitle: a\r\ncreatedAt: 2024-03-03T03:03:03Z\r\nstatus: x\r\n---\r\nbody\r\n", true},
		{"no title", "---\nid: AAAAAA\nstatus: x\n---\n",
			"---\nid: AAAAAA\nstatus: x\ncreatedAt: 2024-03-03T03:03:03Z\n---\n", true},
		{"block title", "---\nid: AAAAAA\ntitle: |\n  long\n---\n",
			"---\nid: AAAAAA\ntitle: |\n  long\ncreatedAt: 2024-03-03T03:03:03Z\n---\n", true},
		{"already has createdAt", "---\nid: AAAAAA\ncreatedAt: x\n---\n", "", false},
		{"leading blank line", "\n---\nid: AAAAAA\n---\n", "", false},
		{"no closing delimiter", "---\nid: AAAAAA\n", "", false},
		{"no frontmatter", "no frontmatter\n", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := insertCreatedAtLine([]byte(tc.in), created)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && string(got) != tc.want {
				t.Errorf("got\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func TestInsideGitWorkTree(t *testing.T) {
	plain := t.TempDir()
	if insideGitWorkTree(plain) {
		t.Error("plain dir reported inside a work tree")
	}

	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(repo, "sub")
	if err := os.Mkdir(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	if !insideGitWorkTree(sub) {
		t.Error("subdir of a .git directory's parent not reported inside a work tree")
	}

	worktree := t.TempDir()
	writeRaw(t, filepath.Join(worktree, ".git"), "gitdir: /elsewhere\n")
	if !insideGitWorkTree(worktree) {
		t.Error(".git file not recognized")
	}

	t.Chdir(plain)
	if insideGitWorkTree(".") {
		t.Error(`"." in a plain dir reported inside a work tree`)
	}
}

func TestStampPolicyFor(t *testing.T) {
	plain := t.TempDir()
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	history := map[string]time.Time{}
	for _, tc := range []struct {
		name    string
		on      bool
		dir     string
		history map[string]time.Time
		gitDir  string
		want    stampPolicy
	}{
		{"option off", false, plain, history, "", stampNone},
		{"history fetched", true, repo, history, "", stampUncommitted},
		{"repo, history unreadable", true, repo, nil, "", stampNone},
		{"GIT_DIR set, history unreadable", true, plain, nil, "/somewhere/.git", stampNone},
		{"not a repo", true, plain, nil, "", stampAll},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GIT_DIR", tc.gitDir)
			if tc.gitDir == "" {
				if err := os.Unsetenv("GIT_DIR"); err != nil {
					t.Fatal(err)
				}
			}
			s := &TikiStore{dir: tc.dir, stampOnLoad: tc.on}
			if got := s.stampPolicyFor(tc.history); got != tc.want {
				t.Errorf("policy = %v, want %v", got, tc.want)
			}
		})
	}
}

func repoWithUncommittedTiki(t *testing.T) (repo, committed, uncommitted string) {
	t.Helper()
	repo = t.TempDir()
	initRepo(t, repo)
	committed = filepath.Join(repo, "c.md")
	writeDoc(t, committed, "CCCCCC", "c")
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "-m", "add c")
	uncommitted = filepath.Join(repo, "n.md")
	writeDoc(t, uncommitted, "NNNNNN", "n")
	setMtime(t, uncommitted, time.Date(2024, 4, 4, 0, 0, 0, 0, time.UTC))
	return repo, committed, uncommitted
}

func TestFullLoad_StampsUncommittedTikiKeepingMtime(t *testing.T) {
	forceBirthTime(t, forcedBirth, true)
	repo, committed, uncommitted := repoWithUncommittedTiki(t)
	committedBefore := readFile(t, committed)

	s := newStoreIn(t, repo, WithCreatedAtStamping())

	want := "---\nid: NNNNNN\ntitle: n\ncreatedAt: 2024-03-03T03:03:03Z\ntype: story\nstatus: inbox\n---\nbody\n"
	if got := readFile(t, uncommitted); got != want {
		t.Errorf("uncommitted file =\n%q\nwant\n%q", got, want)
	}
	april := time.Date(2024, 4, 4, 0, 0, 0, 0, time.UTC)
	if got := mtimeOf(t, uncommitted); !got.Equal(april) {
		t.Errorf("mtime = %v, want it kept at %v", got, april)
	}
	if got := readFile(t, committed); got != committedBefore {
		t.Errorf("committed file changed:\n%s", got)
	}
	tk := s.GetTiki("NNNNNN")
	if !tk.CreatedAtStored() || !tk.UpdatedAt().Equal(april) {
		t.Errorf("stored = %v, UpdatedAt = %v; want stored and UpdatedAt %v", tk.CreatedAtStored(), tk.UpdatedAt(), april)
	}
}

func TestFullLoad_WithoutOptionDoesNotStamp(t *testing.T) {
	repo, _, uncommitted := repoWithUncommittedTiki(t)
	before := readFile(t, uncommitted)

	newStoreIn(t, repo)

	if got := readFile(t, uncommitted); got != before {
		t.Errorf("file changed without the option:\n%s", got)
	}
}

func TestFullLoad_StampsEveryTikiOutsideGit(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, filepath.Join(dir, "a.md"), "AAAAAA", "a")
	writeDoc(t, filepath.Join(dir, "b.md"), "BBBBBB", "b")

	newStoreIn(t, dir, WithCreatedAtStamping())

	for _, name := range []string{"a.md", "b.md"} {
		if got := readFile(t, filepath.Join(dir, name)); !createdAtLine.MatchString(got) {
			t.Errorf("%s not stamped:\n%s", name, got)
		}
	}
}

func TestFullLoad_CommittedPathsAreNotStamped(t *testing.T) {
	for _, tc := range []struct {
		name string
		prep func(t *testing.T, repo string) string
	}{
		{"renamed", func(t *testing.T, repo string) string {
			writeDoc(t, filepath.Join(repo, "a.md"), "AAAAAA", "a")
			runGit(t, repo, "add", "-A")
			runGit(t, repo, "commit", "-q", "-m", "add")
			runGit(t, repo, "mv", "a.md", "b.md")
			runGit(t, repo, "commit", "-q", "-m", "rename")
			return filepath.Join(repo, "b.md")
		}},
		{"non-ASCII path", func(t *testing.T, repo string) string {
			path := filepath.Join(repo, "Über", "x.md")
			writeDoc(t, path, "AAAAAA", "a")
			runGit(t, repo, "add", "-A")
			runGit(t, repo, "commit", "-q", "-m", "add")
			return path
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := t.TempDir()
			initRepo(t, repo)
			path := tc.prep(t, repo)
			before := readFile(t, path)

			newStoreIn(t, repo, WithCreatedAtStamping())

			if got := readFile(t, path); got != before {
				t.Errorf("committed file changed:\n%s", got)
			}
		})
	}
}

func TestFullLoad_InvalidStoredCreatedAtIsNotStamped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	writeRaw(t, path, docWithCreatedLine("a", "createdAt: garbage"))
	before := readFile(t, path)

	s := newStoreIn(t, dir, WithCreatedAtStamping())

	if got := readFile(t, path); got != before {
		t.Errorf("file with an invalid createdAt changed:\n%s", got)
	}
	if err := s.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if s.GetTiki("AAAAAA") == nil {
		t.Error("tiki no longer loads")
	}
}

func TestFullLoad_SymlinkIsNotStamped(t *testing.T) {
	target := filepath.Join(t.TempDir(), "t.md")
	writeDoc(t, target, "AAAAAA", "a")
	before := readFile(t, target)
	dir := t.TempDir()
	if err := os.Symlink(target, filepath.Join(dir, "link.md")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	s := newStoreIn(t, dir, WithCreatedAtStamping())

	if s.GetTiki("AAAAAA") == nil {
		t.Fatal("symlinked tiki not loaded, so the test proves nothing")
	}
	if got := readFile(t, target); got != before {
		t.Errorf("symlink target changed:\n%s", got)
	}
}

func TestReload_StampsTikiAddedSinceStartup(t *testing.T) {
	dir := t.TempDir()
	s := newStoreIn(t, dir, WithCreatedAtStamping())
	path := filepath.Join(dir, "a.md")
	writeDoc(t, path, "AAAAAA", "a")

	if err := s.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	if got := readFile(t, path); !createdAtLine.MatchString(got) {
		t.Errorf("not stamped on reload:\n%s", got)
	}
}

func TestReload_NoStampWhenHistoryUnreadable(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir)
	s := newStoreIn(t, dir, WithCreatedAtStamping())
	path := filepath.Join(dir, "a.md")
	writeDoc(t, path, "AAAAAA", "a")
	before := readFile(t, path)

	s.gitUtil = &failingHistoryGit{}
	if err := s.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := readFile(t, path); got != before {
		t.Fatalf("stamped although the history was unreadable:\n%s", got)
	}

	s.gitUtil = &countingGit{}
	if err := s.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := readFile(t, path); !createdAtLine.MatchString(got) {
		t.Errorf("control: not stamped once the history was readable:\n%s", got)
	}
}

func TestLiveReloadPathsDoNotStamp(t *testing.T) {
	dir := t.TempDir()
	s := newStoreIn(t, dir, WithCreatedAtStamping())

	a := filepath.Join(dir, "a.md")
	writeDoc(t, a, "AAAAAA", "a")
	mustReload(t, s, a)
	if strings.Contains(readFile(t, a), "createdAt") {
		t.Errorf("ReloadPaths stamped:\n%s", readFile(t, a))
	}

	writeExternal(t, a, docContent("AAAAAA", "a2"))
	if err := s.ReloadTiki("AAAAAA"); err != nil {
		t.Fatalf("ReloadTiki: %v", err)
	}
	if strings.Contains(readFile(t, a), "createdAt") {
		t.Errorf("ReloadTiki stamped:\n%s", readFile(t, a))
	}

	b := filepath.Join(dir, "b.md")
	writeDoc(t, b, "BBBBBB", "b")
	if _, err := s.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if strings.Contains(readFile(t, b), "createdAt") {
		t.Errorf("Reconcile stamped:\n%s", readFile(t, b))
	}
}

func TestStampCreatedAt_SkipsFileChangedSinceLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	writeDoc(t, path, "AAAAAA", "a")
	tk := newStoreIn(t, dir).GetTiki("AAAAAA")
	external := docContent("AAAAAA", "external")
	writeExternal(t, path, external)

	if err := stampCreatedAt(tk); err == nil {
		t.Error("stampCreatedAt succeeded on a file changed since load")
	}
	if got := readFile(t, path); got != external {
		t.Errorf("file =\n%s\nwant the external content", got)
	}
}

func TestStampCreatedAt_SkipsWhenMtimeCannotBeKept(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	writeDoc(t, path, "AAAAAA", "a")
	before := readFile(t, path)
	tk := newStoreIn(t, dir).GetTiki("AAAAAA")
	prev := chtimes
	chtimes = func(string, time.Time, time.Time) error { return os.ErrPermission }
	t.Cleanup(func() { chtimes = prev })

	if err := stampCreatedAt(tk); err == nil {
		t.Error("stampCreatedAt succeeded although the mtime can't be kept")
	}
	if got := readFile(t, path); got != before {
		t.Errorf("file changed:\n%s", got)
	}
}

func TestStamp_NotSeenAsExternalEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	writeDoc(t, path, "AAAAAA", "a")
	s := newStoreIn(t, dir, WithCreatedAtStamping())
	if !createdAtLine.MatchString(readFile(t, path)) {
		t.Fatal("precondition: file not stamped at load")
	}

	if res := mustReload(t, s, path); res.Changed {
		t.Errorf("live reload saw the stamp as a change: %+v", res)
	}
	retitle(t, s)
}

func TestStampedContent_KeepsWrappedTitles(t *testing.T) {
	created := time.Date(2024, 3, 3, 3, 3, 3, 0, time.UTC)
	for _, tc := range []struct{ name, in, want string }{
		{"double-quoted title wrapped at column 0",
			"---\nid: AAAAAA\ntitle: \"A long title that an editor\nwrapped\"\nstatus: x\n---\nbody\n",
			"---\nid: AAAAAA\ntitle: \"A long title that an editor\nwrapped\"\nstatus: x\ncreatedAt: 2024-03-03T03:03:03Z\n---\nbody\n"},
		{"single-quoted title wrapped at column 0",
			"---\nid: AAAAAA\ntitle: 'A long title that an editor\nwrapped'\n---\nbody\n",
			"---\nid: AAAAAA\ntitle: 'A long title that an editor\nwrapped'\ncreatedAt: 2024-03-03T03:03:03Z\n---\nbody\n"},
		{"sequence title at column 0",
			"---\nid: AAAAAA\ntitle:\n- a\n---\n",
			"---\nid: AAAAAA\ntitle:\n- a\ncreatedAt: 2024-03-03T03:03:03Z\n---\n"},
		{"single-line title",
			"---\nid: AAAAAA\ntitle: a\nstatus: x\n---\n",
			"---\nid: AAAAAA\ntitle: a\ncreatedAt: 2024-03-03T03:03:03Z\nstatus: x\n---\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := stampedContent([]byte(tc.in), created)
			if !ok {
				t.Fatal("stampedContent refused")
			}
			if string(got) != tc.want {
				t.Errorf("got\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func TestFullLoad_WrappedTitleSurvivesStamp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	writeRaw(t, path, "---\nid: AAAAAA\ntitle: \"A long title that an editor\nwrapped\"\n---\nbody\n")

	s := newStoreIn(t, dir, WithCreatedAtStamping())
	if err := s.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	tk := s.GetTiki("AAAAAA")
	if tk == nil {
		t.Fatal("tiki no longer loads")
	}
	if tk.Title() != "A long title that an editor wrapped" || !tk.CreatedAtStored() {
		t.Errorf("title = %q, stored = %v; want the original title and a stored createdAt", tk.Title(), tk.CreatedAtStored())
	}
}

// A repository below the project has a history the load never reads,
// so its committed files would look uncommitted.
func TestFullLoad_NestedRepoIsNotStamped(t *testing.T) {
	for _, tc := range []struct {
		name      string
		outerRepo bool
		gitFile   bool
	}{
		{"nested repo in a plain folder", false, false},
		{"nested repo in a repo", true, false},
		{"submodule-style .git file", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.outerRepo {
				initRepo(t, dir)
			}
			inner := filepath.Join(dir, "inner")
			nested := filepath.Join(inner, "a.md")
			writeDoc(t, nested, "AAAAAA", "a")
			if tc.gitFile {
				writeRaw(t, filepath.Join(inner, ".git"), "gitdir: ../.git/modules/inner\n")
			} else {
				initRepo(t, inner)
				runGit(t, inner, "add", "-A")
				runGit(t, inner, "commit", "-q", "-m", "add")
			}
			top := filepath.Join(dir, "top.md")
			writeDoc(t, top, "TTTTTT", "top")
			before := readFile(t, nested)

			newStoreIn(t, dir, WithCreatedAtStamping())

			if got := readFile(t, nested); got != before {
				t.Errorf("tiki in the nested repo changed:\n%s", got)
			}
			if !createdAtLine.MatchString(readFile(t, top)) {
				t.Error("control: the project's own uncommitted tiki was not stamped")
			}
		})
	}
}
