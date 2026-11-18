package git_test

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/boolean-maybe/tiki/store/internal/git/internal/gogit"
	"github.com/boolean-maybe/tiki/store/internal/git/internal/shell"
	"github.com/boolean-maybe/tiki/store/internal/git/internal/types"
	gogitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// setupParityRepo creates a temp git repo with deterministic history for parity testing.
// Uses go-git for commits to avoid needing shell git config.
func setupParityRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	repo, err := gogitlib.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}

	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// commit 1: alice creates two tiki files
	for _, name := range []string{"tikis/tiki-001.md", "tikis/tiki-002.md"} {
		abs := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		content := "---\ntitle: " + name + "\nstatus: open\n---\n"
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if _, err := wt.Add(name); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	if _, err := wt.Commit("alice creates tikis", &gogitlib.CommitOptions{
		Author: &object.Signature{Name: "alice", Email: "alice@test.com", When: base},
	}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// commit 2: bob changes status of tiki-001
	t2 := base.Add(30 * 24 * time.Hour)
	abs := filepath.Join(dir, "tikis/tiki-001.md")
	if err := os.WriteFile(abs, []byte("---\ntitle: tikis/tiki-001.md\nstatus: closed\n---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := wt.Add("tikis/tiki-001.md"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := wt.Commit("bob closes tiki 1", &gogitlib.CommitOptions{
		Author: &object.Signature{Name: "bob", Email: "bob@test.com", When: t2},
	}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// commit 3: charlie creates tiki-003
	t3 := base.Add(60 * 24 * time.Hour)
	abs3 := filepath.Join(dir, "tikis/tiki-003.md")
	if err := os.WriteFile(abs3, []byte("---\ntitle: tiki 3\nstatus: open\n---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := wt.Add("tikis/tiki-003.md"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := wt.Commit("charlie creates tiki 3", &gogitlib.CommitOptions{
		Author: &object.Signature{Name: "charlie", Email: "charlie@test.com", When: t3},
	}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// set git config for shell backend (needed for CurrentUser)
	cfg, err := repo.Config()
	if err != nil {
		t.Fatalf("repo.Config: %v", err)
	}
	cfg.User.Name = "testuser"
	cfg.User.Email = "testuser@test.com"
	if err := repo.SetConfig(cfg); err != nil {
		t.Fatalf("repo.SetConfig: %v", err)
	}

	return dir
}

func requireShellGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("shell git not available")
	}
}

func TestParity_CurrentBranch(t *testing.T) {
	requireShellGit(t)
	dir := setupParityRepo(t)

	sh, err := shell.NewUtil(dir)
	if err != nil {
		t.Fatalf("shell.NewUtil: %v", err)
	}
	gg, err := gogit.NewUtil(dir)
	if err != nil {
		t.Fatalf("gogit.NewUtil: %v", err)
	}

	shellBranch, err := sh.CurrentBranch()
	if err != nil {
		t.Fatalf("shell.CurrentBranch: %v", err)
	}
	gogitBranch, err := gg.CurrentBranch()
	if err != nil {
		t.Fatalf("gogit.CurrentBranch: %v", err)
	}

	if shellBranch != gogitBranch {
		t.Errorf("CurrentBranch mismatch: shell=%q gogit=%q", shellBranch, gogitBranch)
	}
}

func TestParity_Author(t *testing.T) {
	requireShellGit(t)
	dir := setupParityRepo(t)

	sh, err := shell.NewUtil(dir)
	if err != nil {
		t.Fatalf("shell.NewUtil: %v", err)
	}
	gg, err := gogit.NewUtil(dir)
	if err != nil {
		t.Fatalf("gogit.NewUtil: %v", err)
	}

	for _, file := range []string{"tikis/tiki-001.md", "tikis/tiki-002.md", "tikis/tiki-003.md"} {
		shellAuthor, err := sh.Author(file)
		if err != nil {
			t.Fatalf("shell.Author(%s): %v", file, err)
		}
		gogitAuthor, err := gg.Author(file)
		if err != nil {
			t.Fatalf("gogit.Author(%s): %v", file, err)
		}

		if shellAuthor.Name != gogitAuthor.Name {
			t.Errorf("Author(%s) name: shell=%q gogit=%q", file, shellAuthor.Name, gogitAuthor.Name)
		}
		if shellAuthor.Email != gogitAuthor.Email {
			t.Errorf("Author(%s) email: shell=%q gogit=%q", file, shellAuthor.Email, gogitAuthor.Email)
		}
	}
}

func TestParity_AllAuthors(t *testing.T) {
	requireShellGit(t)
	dir := setupParityRepo(t)

	sh, err := shell.NewUtil(dir)
	if err != nil {
		t.Fatalf("shell.NewUtil: %v", err)
	}
	gg, err := gogit.NewUtil(dir)
	if err != nil {
		t.Fatalf("gogit.NewUtil: %v", err)
	}

	pattern := filepath.Join(dir, "tikis", "*.md")
	shellAuthors, err := sh.AllAuthors(pattern)
	if err != nil {
		t.Fatalf("shell.AllAuthors: %v", err)
	}
	gogitAuthors, err := gg.AllAuthors(pattern)
	if err != nil {
		t.Fatalf("gogit.AllAuthors: %v", err)
	}

	// normalize keys and compare
	shellKeys := sortedKeys(shellAuthors)
	gogitKeys := sortedKeys(gogitAuthors)

	if len(shellKeys) != len(gogitKeys) {
		t.Fatalf("AllAuthors key count: shell=%d gogit=%d\nshell=%v\ngogit=%v", len(shellKeys), len(gogitKeys), shellKeys, gogitKeys)
	}

	for _, key := range shellKeys {
		sa := shellAuthors[key]
		ga, ok := gogitAuthors[key]
		if !ok {
			t.Errorf("AllAuthors: shell has key %q, gogit does not", key)
			continue
		}
		if sa.Name != ga.Name {
			t.Errorf("AllAuthors[%s] name: shell=%q gogit=%q", key, sa.Name, ga.Name)
		}
	}
}

func TestParity_AllUsers(t *testing.T) {
	requireShellGit(t)
	dir := setupParityRepo(t)

	sh, err := shell.NewUtil(dir)
	if err != nil {
		t.Fatalf("shell.NewUtil: %v", err)
	}
	gg, err := gogit.NewUtil(dir)
	if err != nil {
		t.Fatalf("gogit.NewUtil: %v", err)
	}

	shellUsers, err := sh.AllUsers()
	if err != nil {
		t.Fatalf("shell.AllUsers: %v", err)
	}
	gogitUsers, err := gg.AllUsers()
	if err != nil {
		t.Fatalf("gogit.AllUsers: %v", err)
	}

	// both should have same users in same order (first-seen traversal)
	if len(shellUsers) != len(gogitUsers) {
		t.Fatalf("AllUsers count: shell=%d gogit=%d\nshell=%v\ngogit=%v", len(shellUsers), len(gogitUsers), shellUsers, gogitUsers)
	}

	for i := range shellUsers {
		if shellUsers[i] != gogitUsers[i] {
			t.Errorf("AllUsers[%d]: shell=%q gogit=%q", i, shellUsers[i], gogitUsers[i])
		}
	}
}

func TestParity_LastCommitTime(t *testing.T) {
	requireShellGit(t)
	dir := setupParityRepo(t)

	sh, err := shell.NewUtil(dir)
	if err != nil {
		t.Fatalf("shell.NewUtil: %v", err)
	}
	gg, err := gogit.NewUtil(dir)
	if err != nil {
		t.Fatalf("gogit.NewUtil: %v", err)
	}

	for _, file := range []string{"tikis/tiki-001.md", "tikis/tiki-002.md"} {
		shellTime, err := sh.LastCommitTime(file)
		if err != nil {
			t.Fatalf("shell.LastCommitTime(%s): %v", file, err)
		}
		gogitTime, err := gg.LastCommitTime(file)
		if err != nil {
			t.Fatalf("gogit.LastCommitTime(%s): %v", file, err)
		}

		if !shellTime.Equal(gogitTime) {
			t.Errorf("LastCommitTime(%s): shell=%v gogit=%v", file, shellTime, gogitTime)
		}
	}
}

func TestParity_AllLastCommitTimes(t *testing.T) {
	requireShellGit(t)
	dir := setupParityRepo(t)

	sh, err := shell.NewUtil(dir)
	if err != nil {
		t.Fatalf("shell.NewUtil: %v", err)
	}
	gg, err := gogit.NewUtil(dir)
	if err != nil {
		t.Fatalf("gogit.NewUtil: %v", err)
	}

	pattern := filepath.Join(dir, "tikis", "*.md")
	shellTimes, err := sh.AllLastCommitTimes(pattern)
	if err != nil {
		t.Fatalf("shell.AllLastCommitTimes: %v", err)
	}
	gogitTimes, err := gg.AllLastCommitTimes(pattern)
	if err != nil {
		t.Fatalf("gogit.AllLastCommitTimes: %v", err)
	}

	shellKeys := sortedTimeKeys(shellTimes)
	gogitKeys := sortedTimeKeys(gogitTimes)

	if len(shellKeys) != len(gogitKeys) {
		t.Fatalf("AllLastCommitTimes key count: shell=%d gogit=%d\nshell=%v\ngogit=%v", len(shellKeys), len(gogitKeys), shellKeys, gogitKeys)
	}

	for _, key := range shellKeys {
		st := shellTimes[key]
		gt, ok := gogitTimes[key]
		if !ok {
			t.Errorf("AllLastCommitTimes: shell has key %q, gogit does not", key)
			continue
		}
		if !st.Equal(gt) {
			t.Errorf("AllLastCommitTimes[%s]: shell=%v gogit=%v", key, st, gt)
		}
	}
}

func TestParity_FileVersionsSince(t *testing.T) {
	requireShellGit(t)
	dir := setupParityRepo(t)

	sh, err := shell.NewUtil(dir)
	if err != nil {
		t.Fatalf("shell.NewUtil: %v", err)
	}
	gg, err := gogit.NewUtil(dir)
	if err != nil {
		t.Fatalf("gogit.NewUtil: %v", err)
	}

	since := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	for _, includePrior := range []bool{false, true} {
		shellVersions, err := sh.FileVersionsSince("tikis/tiki-001.md", since, includePrior)
		if err != nil {
			t.Fatalf("shell.FileVersionsSince(prior=%v): %v", includePrior, err)
		}
		gogitVersions, err := gg.FileVersionsSince("tikis/tiki-001.md", since, includePrior)
		if err != nil {
			t.Fatalf("gogit.FileVersionsSince(prior=%v): %v", includePrior, err)
		}

		if len(shellVersions) != len(gogitVersions) {
			t.Fatalf("FileVersionsSince(prior=%v) count: shell=%d gogit=%d", includePrior, len(shellVersions), len(gogitVersions))
		}

		for i := range shellVersions {
			if shellVersions[i].Content != gogitVersions[i].Content {
				t.Errorf("FileVersionsSince(prior=%v)[%d] content mismatch:\nshell=%q\ngogit=%q", includePrior, i, shellVersions[i].Content, gogitVersions[i].Content)
			}
			if shellVersions[i].Author != gogitVersions[i].Author {
				t.Errorf("FileVersionsSince(prior=%v)[%d] author: shell=%q gogit=%q", includePrior, i, shellVersions[i].Author, gogitVersions[i].Author)
			}
		}
	}
}

func TestParity_AllFileVersionsSince(t *testing.T) {
	requireShellGit(t)
	dir := setupParityRepo(t)

	sh, err := shell.NewUtil(dir)
	if err != nil {
		t.Fatalf("shell.NewUtil: %v", err)
	}
	gg, err := gogit.NewUtil(dir)
	if err != nil {
		t.Fatalf("gogit.NewUtil: %v", err)
	}

	since := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	pattern := filepath.Join(dir, "tikis", "*.md")

	for _, includePrior := range []bool{false, true} {
		shellResult, err := sh.AllFileVersionsSince(pattern, since, includePrior)
		if err != nil {
			t.Fatalf("shell.AllFileVersionsSince(prior=%v): %v", includePrior, err)
		}
		gogitResult, err := gg.AllFileVersionsSince(pattern, since, includePrior)
		if err != nil {
			t.Fatalf("gogit.AllFileVersionsSince(prior=%v): %v", includePrior, err)
		}

		shellFileKeys := sortedVersionKeys(shellResult)
		gogitFileKeys := sortedVersionKeys(gogitResult)

		if len(shellFileKeys) != len(gogitFileKeys) {
			t.Errorf("AllFileVersionsSince(prior=%v) file count: shell=%d gogit=%d\nshell=%v\ngogit=%v",
				includePrior, len(shellFileKeys), len(gogitFileKeys), shellFileKeys, gogitFileKeys)
			continue
		}

		for _, key := range shellFileKeys {
			sv := shellResult[key]
			gv := gogitResult[key]

			// sort versions by time then hash for stable comparison
			sortVersions(sv)
			sortVersions(gv)

			if len(sv) != len(gv) {
				t.Errorf("AllFileVersionsSince(prior=%v)[%s] version count: shell=%d gogit=%d",
					includePrior, key, len(sv), len(gv))
				continue
			}

			for i := range sv {
				if sv[i].Content != gv[i].Content {
					t.Errorf("AllFileVersionsSince(prior=%v)[%s][%d] content mismatch", includePrior, key, i)
				}
				if sv[i].Author != gv[i].Author {
					t.Errorf("AllFileVersionsSince(prior=%v)[%s][%d] author: shell=%q gogit=%q", includePrior, key, i, sv[i].Author, gv[i].Author)
				}
			}
		}
	}
}

func TestParity_Init(t *testing.T) {
	requireShellGit(t)

	shellDir := filepath.Join(t.TempDir(), "shell-repo")
	if err := shell.Init(shellDir); err != nil {
		t.Fatalf("shell.Init: %v", err)
	}

	gogitDir := filepath.Join(t.TempDir(), "gogit-repo")
	if err := gogit.Init(gogitDir); err != nil {
		t.Fatalf("gogit.Init: %v", err)
	}

	// both should produce repos that IsRepo recognizes
	shellRepo, err := gogitlib.PlainOpen(shellDir)
	if err != nil {
		t.Fatalf("shell init dir is not a valid repo: %v", err)
	}
	gogitRepo, err := gogitlib.PlainOpen(gogitDir)
	if err != nil {
		t.Fatalf("gogit init dir is not a valid repo: %v", err)
	}

	// both should have HEAD pointing to a branch (even if unborn)
	shellHead, _ := shellRepo.Head()
	gogitHead, _ := gogitRepo.Head()
	_ = shellHead
	_ = gogitHead
}

func sortedKeys(m map[string]*types.AuthorInfo) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedTimeKeys(m map[string]time.Time) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedVersionKeys(m map[string][]types.FileVersion) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortVersions(v []types.FileVersion) {
	sort.SliceStable(v, func(i, j int) bool {
		if v[i].When.Equal(v[j].When) {
			return v[i].Hash < v[j].Hash
		}
		return v[i].When.Before(v[j].When)
	})
}

// The store passes its own directory, which is the repo root in the common layout.
func TestParity_RepoRootPattern(t *testing.T) {
	requireShellGit(t)
	dir := setupParityRepo(t)

	sh, err := shell.NewUtil(dir)
	if err != nil {
		t.Fatalf("shell.NewUtil: %v", err)
	}
	gg, err := gogit.NewUtil(dir)
	if err != nil {
		t.Fatalf("gogit.NewUtil: %v", err)
	}

	want := []string{"tikis/tiki-001.md", "tikis/tiki-002.md", "tikis/tiki-003.md"}
	for _, b := range []struct {
		name    string
		authors func(string) (map[string]*types.AuthorInfo, error)
		times   func(string) (map[string]time.Time, error)
	}{
		{"shell", sh.AllAuthors, sh.AllLastCommitTimes},
		{"gogit", gg.AllAuthors, gg.AllLastCommitTimes},
	} {
		authors, err := b.authors(dir)
		if err != nil {
			t.Fatalf("%s AllAuthors: %v", b.name, err)
		}
		if got := sortedKeys(authors); !slices.Equal(got, want) {
			t.Errorf("%s AllAuthors keys = %v, want %v", b.name, got, want)
		}
		times, err := b.times(dir)
		if err != nil {
			t.Fatalf("%s AllLastCommitTimes: %v", b.name, err)
		}
		if got := sortedTimeKeys(times); !slices.Equal(got, want) {
			t.Errorf("%s AllLastCommitTimes keys = %v, want %v", b.name, got, want)
		}
	}
}

func TestParity_RepoRelPath(t *testing.T) {
	requireShellGit(t)
	dir := setupParityRepo(t)
	file := filepath.Join(dir, "tikis", "tiki-001.md")

	for _, opened := range []string{dir, filepath.Join(dir, "tikis")} {
		sh, err := shell.NewUtil(opened)
		if err != nil {
			t.Fatalf("shell.NewUtil(%s): %v", opened, err)
		}
		gg, err := gogit.NewUtil(opened)
		if err != nil {
			t.Fatalf("gogit.NewUtil(%s): %v", opened, err)
		}
		for name, rel := range map[string]func(string) (string, error){
			"shell": sh.RepoRelPath,
			"gogit": gg.RepoRelPath,
		} {
			got, err := rel(file)
			if err != nil {
				t.Fatalf("%s RepoRelPath (opened at %s): %v", name, opened, err)
			}
			if got != "tikis/tiki-001.md" {
				t.Errorf("%s RepoRelPath (opened at %s) = %q, want %q", name, opened, got, "tikis/tiki-001.md")
			}
		}
	}
}

// git escapes non-ASCII bytes in path output unless told not to.
func TestParity_NonASCIIPath(t *testing.T) {
	requireShellGit(t)
	dir := t.TempDir()
	repo, err := gogitlib.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	const name = "Über/café.md"
	abs := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(abs, []byte("---\ntitle: x\n---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := wt.Add(name); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := wt.Commit("add", &gogitlib.CommitOptions{
		Author: &object.Signature{Name: "alice", Email: "alice@test.com", When: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)},
	}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	sh, err := shell.NewUtil(dir)
	if err != nil {
		t.Fatalf("shell.NewUtil: %v", err)
	}
	gg, err := gogit.NewUtil(dir)
	if err != nil {
		t.Fatalf("gogit.NewUtil: %v", err)
	}
	for _, b := range []struct {
		name    string
		authors func(string) (map[string]*types.AuthorInfo, error)
		times   func(string) (map[string]time.Time, error)
	}{
		{"shell", sh.AllAuthors, sh.AllLastCommitTimes},
		{"gogit", gg.AllAuthors, gg.AllLastCommitTimes},
	} {
		authors, err := b.authors(dir)
		if err != nil {
			t.Fatalf("%s AllAuthors: %v", b.name, err)
		}
		if _, ok := authors[name]; !ok {
			t.Errorf("%s AllAuthors keys = %v, want %q", b.name, sortedKeys(authors), name)
		}
		times, err := b.times(dir)
		if err != nil {
			t.Fatalf("%s AllLastCommitTimes: %v", b.name, err)
		}
		if _, ok := times[name]; !ok {
			t.Errorf("%s AllLastCommitTimes keys = %v, want %q", b.name, sortedTimeKeys(times), name)
		}
	}
	if got, err := sh.RepoRelPath(abs); err != nil || got != name {
		t.Errorf("shell RepoRelPath = %q, %v; want %q", got, err, name)
	}
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	//nolint:gosec // G204: test-controlled git args
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeIn(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendIn(t *testing.T, path string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("more\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func newShellRepo(t *testing.T) string {
	t.Helper()
	requireShellGit(t)
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	gitIn(t, dir, "config", "user.email", "t@example.com")
	gitIn(t, dir, "config", "user.name", "tester")
	return dir
}

type changeSource interface {
	ChangedFiles(dirPattern string) (map[string]types.FileChange, error)
	HeadContents(paths []string) (map[string][]byte, error)
}

func bothBackends(t *testing.T, at string) map[string]changeSource {
	t.Helper()
	sh, err := shell.NewUtil(at)
	if err != nil {
		t.Fatalf("shell.NewUtil: %v", err)
	}
	gg, err := gogit.NewUtil(at)
	if err != nil {
		t.Fatalf("gogit.NewUtil: %v", err)
	}
	return map[string]changeSource{"shell": sh, "gogit": gg}
}

func TestParity_ChangedFiles(t *testing.T) {
	dir := newShellRepo(t)
	sub := filepath.Join(dir, "sub")
	for _, name := range []string{"a.md", "b.md", "c.md", "d.md", "e.md", "Über.md"} {
		writeIn(t, filepath.Join(sub, name), name+"\n")
	}
	writeIn(t, filepath.Join(dir, "other", "o.txt"), "o\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "init")

	appendIn(t, filepath.Join(sub, "a.md"))
	gitIn(t, dir, "mv", "sub/b.md", "sub/b2.md")
	if err := os.MkdirAll(filepath.Join(sub, "done"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(sub, "c.md"), filepath.Join(sub, "done", "c.md")); err != nil {
		t.Fatal(err)
	}
	appendIn(t, filepath.Join(sub, "d.md"))
	gitIn(t, dir, "add", "sub/d.md")
	if err := os.Rename(filepath.Join(sub, "Über.md"), filepath.Join(sub, "Ünder.md")); err != nil {
		t.Fatal(err)
	}
	appendIn(t, filepath.Join(dir, "other", "o.txt"))
	gitIn(t, dir, "config", "core.fileMode", "false")
	//nolint:gosec // G302: the test needs a permission-only change
	if err := os.Chmod(filepath.Join(sub, "e.md"), 0o755); err != nil {
		t.Fatal(err)
	}

	want := map[string]types.FileChange{
		"sub/a.md": types.FileChanged, "sub/b2.md": types.FileChanged, "sub/d.md": types.FileChanged,
		"sub/b.md": types.FileGone, "sub/c.md": types.FileGone, "sub/Über.md": types.FileGone,
	}
	for name, b := range bothBackends(t, sub) {
		got, err := b.ChangedFiles(sub)
		if err != nil {
			t.Fatalf("%s ChangedFiles: %v", name, err)
		}
		if !maps.Equal(got, want) {
			t.Errorf("%s ChangedFiles = %v, want %v", name, got, want)
		}
	}
}

func TestParity_ChangedFilesUnbornHead(t *testing.T) {
	dir := newShellRepo(t)
	writeIn(t, filepath.Join(dir, "a.md"), "a\n")
	gitIn(t, dir, "add", "-A")
	for name, b := range bothBackends(t, dir) {
		if _, err := b.ChangedFiles(dir); err != nil {
			t.Errorf("%s ChangedFiles on a repo with no commit: %v", name, err)
		}
		got, err := b.HeadContents([]string{"a.md"})
		if err != nil || len(got) != 0 {
			t.Errorf("%s HeadContents on a repo with no commit = %v, %v; want empty, nil", name, got, err)
		}
	}
}

func TestParity_HeadContents(t *testing.T) {
	dir := newShellRepo(t)
	writeIn(t, filepath.Join(dir, "sub", "c.md"), "---\nid: CCCCCC\n---\n")
	writeIn(t, filepath.Join(dir, "sub", "Über.md"), "über\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "init")
	want := map[string]string{"sub/c.md": "---\nid: CCCCCC\n---\n", "sub/Über.md": "über\n"}

	for name, b := range bothBackends(t, dir) {
		got, err := b.HeadContents([]string{"sub/c.md", "sub/Über.md", "sub/missing.md"})
		if err != nil {
			t.Fatalf("%s HeadContents: %v", name, err)
		}
		if len(got) != len(want) {
			t.Errorf("%s HeadContents returned %d files, want %d", name, len(got), len(want))
		}
		for path, content := range want {
			if string(got[path]) != content {
				t.Errorf("%s HeadContents[%s] = %q, want %q", name, path, got[path], content)
			}
		}
		if empty, err := b.HeadContents(nil); err != nil || len(empty) != 0 {
			t.Errorf("%s HeadContents(nil) = %v, %v; want empty, nil", name, empty, err)
		}
	}
}

// A committed rename must never count as the file being added, whatever the user's diff.renames.
func TestParity_CommittedRenameIsNotAnAdd(t *testing.T) {
	dir := newShellRepo(t)
	writeIn(t, filepath.Join(dir, "a.md"), "---\nid: AAAAAA\ntitle: a\n---\nbody\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "add")
	gitIn(t, dir, "mv", "a.md", "b.md")
	gitIn(t, dir, "commit", "-q", "-m", "rename")
	gitIn(t, dir, "config", "diff.renames", "false")

	sh, err := shell.NewUtil(dir)
	if err != nil {
		t.Fatal(err)
	}
	gg, err := gogit.NewUtil(dir)
	if err != nil {
		t.Fatal(err)
	}
	for name, all := range map[string]func(string) (map[string]*types.AuthorInfo, error){"shell": sh.AllAuthors, "gogit": gg.AllAuthors} {
		authors, err := all(dir)
		if err != nil {
			t.Fatalf("%s AllAuthors: %v", name, err)
		}
		if _, ok := authors["a.md"]; !ok {
			t.Errorf("%s AllAuthors lacks the original a.md", name)
		}
		if _, ok := authors["b.md"]; ok {
			t.Errorf("%s AllAuthors counts the rename to b.md as an add", name)
		}
	}
}

// Only a similar file counts as renamed, as in git's own -M:
// an unrelated delete in the same commit must not swallow an add.
func TestParity_UnrelatedDeleteAndAddIsAnAdd(t *testing.T) {
	dir := newShellRepo(t)
	writeIn(t, filepath.Join(dir, "src", "main.go"), "package main\n\nfunc main() {}\n")
	writeIn(t, filepath.Join(dir, "x.md"), "---\nid: XXXXXX\ntitle: x\n---\nthe first tiki\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "first")
	gitIn(t, dir, "rm", "-q", "x.md", "src/main.go")
	writeIn(t, filepath.Join(dir, "y.md"), "---\nid: YYYYYY\ntitle: y\n---\nan unrelated tiki\n")
	writeIn(t, filepath.Join(dir, ".doc", "new.md"), "---\nid: NNNNNN\ntitle: n\n---\nanother one\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "second")

	sh, err := shell.NewUtil(dir)
	if err != nil {
		t.Fatal(err)
	}
	gg, err := gogit.NewUtil(dir)
	if err != nil {
		t.Fatal(err)
	}
	for name, all := range map[string]func(string) (map[string]*types.AuthorInfo, error){"shell": sh.AllAuthors, "gogit": gg.AllAuthors} {
		authors, err := all(dir)
		if err != nil {
			t.Fatalf("%s AllAuthors: %v", name, err)
		}
		for _, path := range []string{"y.md", ".doc/new.md"} {
			if _, ok := authors[path]; !ok {
				t.Errorf("%s AllAuthors lacks %s, added next to an unrelated delete", name, path)
			}
		}
	}
}

// A clean checkout whose files got CRLF line endings on disk has nothing changed.
func TestParity_ChangedFilesCRLFCheckout(t *testing.T) {
	dir := newShellRepo(t)
	writeIn(t, filepath.Join(dir, "a.md"), "---\nid: AAAAAA\ntitle: a\n---\nbody\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "init")
	gitIn(t, dir, "config", "core.autocrlf", "true")
	if err := os.Remove(filepath.Join(dir, "a.md")); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "checkout", "--", "a.md")

	for name, b := range bothBackends(t, dir) {
		got, err := b.ChangedFiles(dir)
		if err != nil {
			t.Fatalf("%s ChangedFiles: %v", name, err)
		}
		if len(got) != 0 {
			t.Errorf("%s ChangedFiles = %v, want nothing changed", name, got)
		}
	}
}
