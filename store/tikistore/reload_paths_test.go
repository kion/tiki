package tikistore

import (
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	gitops "github.com/boolean-maybe/tiki/store/internal/git"
	tikipkg "github.com/boolean-maybe/tiki/tiki"
)

type countingGit struct {
	mu      sync.Mutex
	history int
}

func (g *countingGit) bump() {
	g.mu.Lock()
	g.history++
	g.mu.Unlock()
}

func (g *countingGit) historyCalls() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.history
}

func (g *countingGit) Add(...string) error    { return nil }
func (g *countingGit) Remove(...string) error { return nil }
func (g *countingGit) CurrentUser() (string, string, error) {
	return "tester", "tester@example.com", nil
}
func (g *countingGit) Author(string) (*gitops.AuthorInfo, error) {
	g.bump()
	return nil, os.ErrNotExist
}
func (g *countingGit) AllAuthors(string) (map[string]*gitops.AuthorInfo, error) {
	g.bump()
	return map[string]*gitops.AuthorInfo{}, nil
}
func (g *countingGit) LastCommitTime(string) (time.Time, error) {
	g.bump()
	return time.Time{}, os.ErrNotExist
}
func (g *countingGit) AllLastCommitTimes(string) (map[string]time.Time, error) {
	g.bump()
	return map[string]time.Time{}, nil
}
func (g *countingGit) CurrentBranch() (string, error) { return "main", nil }
func (g *countingGit) FileVersionsSince(string, time.Time, bool) ([]gitops.FileVersion, error) {
	g.bump()
	return nil, nil
}
func (g *countingGit) AllFileVersionsSince(string, time.Time, bool) (map[string][]gitops.FileVersion, error) {
	g.bump()
	return nil, nil
}
func (g *countingGit) AllUsers() ([]string, error) { return nil, nil }

func newReloadStore(t *testing.T, root string) (*TikiStore, *countingGit, *int) {
	t.Helper()
	s, err := NewTikiStore(root)
	if err != nil {
		t.Fatalf("NewTikiStore: %v", err)
	}
	g := &countingGit{}
	s.gitUtil = g
	s.identity = newIdentityResolver(g)
	notified := 0
	s.AddListener(func() { notified++ })
	return s, g, &notified
}

// The mtime is pushed forward,
// so the change is visible even with coarse timestamps.
func writeExternal(t *testing.T, path, content string) {
	t.Helper()
	//nolint:gosec // G301: test temp dir
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	//nolint:gosec // G306: test temp file
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
}

func docContent(id, title string) string {
	return "---\nid: " + id + "\ntitle: " + title + "\ntype: story\nstatus: inbox\n---\nbody\n"
}

func mustReload(t *testing.T, s *TikiStore, paths ...string) ReloadResult {
	t.Helper()
	res, err := s.ReloadPaths(paths)
	if err != nil {
		t.Fatalf("ReloadPaths: %v", err)
	}
	return res
}

func sortedIDs(ids []string) []string {
	out := append([]string(nil), ids...)
	sort.Strings(out)
	return out
}

func TestReloadPaths_SelfWriteIsNoop(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.md")
	writeDoc(t, path, "AAAAAA", "original")
	s, _, notified := newReloadStore(t, root)

	updated := s.GetTiki("AAAAAA").Clone()
	updated.SetTitle("edited in app")
	if err := s.UpdateTiki(updated); err != nil {
		t.Fatalf("UpdateTiki: %v", err)
	}
	before := s.GetTiki("AAAAAA")
	*notified = 0

	res := mustReload(t, s, path)

	if res.Changed {
		t.Errorf("self-written file reported as changed: %+v", res)
	}
	if *notified != 0 {
		t.Errorf("listeners notified %d times for a self-write, want 0", *notified)
	}
	if s.GetTiki("AAAAAA") != before {
		t.Error("self-write replaced the in-memory tiki")
	}
}

func TestReloadPaths_AddsNewNestedFile(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, filepath.Join(root, "a.md"), "AAAAAA", "a")
	s, _, notified := newReloadStore(t, root)

	newPath := filepath.Join(root, "sub", "deeper", "my-new-thing.md")
	writeExternal(t, newPath, docContent("NEWNEW", "new thing"))

	res := mustReload(t, s, newPath)

	if got := sortedIDs(res.Added); len(got) != 1 || got[0] != "NEWNEW" {
		t.Errorf("Added = %v, want [NEWNEW]", got)
	}
	tk := s.GetTiki("NEWNEW")
	if tk == nil {
		t.Fatal("new tiki not loaded")
	}
	if tk.Path() != newPath {
		t.Errorf("Path = %q, want %q", tk.Path(), newPath)
	}
	if *notified != 1 {
		t.Errorf("notified %d times, want 1", *notified)
	}
}

func TestReloadPaths_ModifyCarriesGitMetadata(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.md")
	writeDoc(t, path, "AAAAAA", "original")
	s, _, _ := newReloadStore(t, root)

	created := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	old := s.GetTiki("AAAAAA")
	old.SetCreatedAt(created)
	old.Set("createdBy", "Original Author")

	writeExternal(t, path, docContent("AAAAAA", "edited elsewhere"))
	res := mustReload(t, s, path)

	if got := sortedIDs(res.Updated); len(got) != 1 || got[0] != "AAAAAA" {
		t.Fatalf("Updated = %v, want [AAAAAA]", got)
	}
	tk := s.GetTiki("AAAAAA")
	if tk == old {
		t.Fatal("modified tiki kept its old pointer")
	}
	if tk.Title() != "edited elsewhere" {
		t.Errorf("Title = %q, want %q", tk.Title(), "edited elsewhere")
	}
	if !tk.CreatedAt().Equal(created) {
		t.Errorf("CreatedAt = %v, want carried-over %v", tk.CreatedAt(), created)
	}
	if v, _ := tk.Get("createdBy"); v != "Original Author" {
		t.Errorf("createdBy = %v, want carried-over %q", v, "Original Author")
	}
}

func TestReloadPaths_UnrelatedTikisKeepPointers(t *testing.T) {
	root := t.TempDir()
	pathA := filepath.Join(root, "a.md")
	writeDoc(t, pathA, "AAAAAA", "a")
	writeDoc(t, filepath.Join(root, "b.md"), "BBBBBB", "b")
	s, _, _ := newReloadStore(t, root)
	b := s.GetTiki("BBBBBB")

	writeExternal(t, pathA, docContent("AAAAAA", "a2"))
	mustReload(t, s, pathA)

	if s.GetTiki("BBBBBB") != b {
		t.Error("unrelated tiki was replaced")
	}
}

func TestReloadPaths_Delete(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.md")
	writeDoc(t, path, "AAAAAA", "a")
	s, _, notified := newReloadStore(t, root)

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	res := mustReload(t, s, path)

	if got := sortedIDs(res.Removed); len(got) != 1 || got[0] != "AAAAAA" {
		t.Errorf("Removed = %v, want [AAAAAA]", got)
	}
	if s.GetTiki("AAAAAA") != nil {
		t.Error("deleted tiki still loaded")
	}
	if *notified != 1 {
		t.Errorf("notified %d times, want 1", *notified)
	}
}

func TestReloadPaths_RenameInOneBatch(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "a.md")
	writeDoc(t, oldPath, "AAAAAA", "a")
	s, _, _ := newReloadStore(t, root)

	newPath := filepath.Join(root, "moved", "renamed.md")
	//nolint:gosec // G301: test temp dir
	if err := os.MkdirAll(filepath.Dir(newPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatal(err)
	}
	res := mustReload(t, s, oldPath, newPath)

	tk := s.GetTiki("AAAAAA")
	if tk == nil || tk.Path() != newPath {
		t.Fatalf("after rename, tiki = %v, want at %s", tk, newPath)
	}
	if !res.Changed {
		t.Error("rename not reported as a change")
	}
}

func TestReloadPaths_RenameAcrossBatches(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "a.md")
	writeDoc(t, oldPath, "AAAAAA", "a")
	s, _, _ := newReloadStore(t, root)

	newPath := filepath.Join(root, "renamed.md")
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatal(err)
	}
	// the create of the new name arrives before the removal of the old one
	mustReload(t, s, newPath)
	if tk := s.GetTiki("AAAAAA"); tk == nil || tk.Path() != newPath {
		t.Fatalf("after first batch, tiki = %v, want at %s", tk, newPath)
	}
	res := mustReload(t, s, oldPath)
	if res.Changed {
		t.Errorf("stale removal of the old name changed the store: %+v", res)
	}
	if tk := s.GetTiki("AAAAAA"); tk == nil || tk.Path() != newPath {
		t.Fatalf("after second batch, tiki = %v, want at %s", tk, newPath)
	}
}

func TestReloadPaths_IDChange(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.md")
	writeDoc(t, path, "AAAAAA", "a")
	s, _, _ := newReloadStore(t, root)

	writeExternal(t, path, docContent("CHANGE", "a"))
	mustReload(t, s, path)

	if s.GetTiki("AAAAAA") != nil {
		t.Error("old id still loaded after id change")
	}
	if tk := s.GetTiki("CHANGE"); tk == nil || tk.Path() != path {
		t.Errorf("new id not loaded at %s: %v", path, tk)
	}
}

func TestReloadPaths_IDCollisionRecordsDiagnostic(t *testing.T) {
	root := t.TempDir()
	pathA := filepath.Join(root, "a.md")
	pathB := filepath.Join(root, "b.md")
	writeDoc(t, pathA, "AAAAAA", "a")
	writeDoc(t, pathB, "BBBBBB", "b")
	s, _, _ := newReloadStore(t, root)

	writeExternal(t, pathB, docContent("AAAAAA", "b now claims a's id"))
	mustReload(t, s, pathB)

	if tk := s.GetTiki("AAAAAA"); tk == nil || tk.Path() != pathA {
		t.Errorf("first path in sorted order must keep the id; got %v", tk)
	}
	if s.GetTiki("BBBBBB") != nil {
		t.Error("BBBBBB should be gone: its file now carries another id")
	}
	rejections := s.LoadDiagnostics().Rejections()
	if len(rejections) != 1 || rejections[0].Path != pathB || rejections[0].Reason != LoadReasonDuplicateID {
		t.Errorf("rejections = %+v, want one duplicate-id rejection for %s", rejections, pathB)
	}
}

func TestReloadPaths_DuplicatePromotedWhenOriginalDeleted(t *testing.T) {
	root := t.TempDir()
	pathA := filepath.Join(root, "a.md")
	pathB := filepath.Join(root, "b.md")
	writeDoc(t, pathA, "AAAAAA", "original")
	writeDoc(t, pathB, "AAAAAA", "duplicate")
	s, _, _ := newReloadStore(t, root)
	if !s.LoadDiagnostics().HasIssues() {
		t.Fatal("precondition: duplicate should be rejected at load")
	}

	if err := os.Remove(pathA); err != nil {
		t.Fatal(err)
	}
	mustReload(t, s, pathA)

	tk := s.GetTiki("AAAAAA")
	if tk == nil || tk.Path() != pathB {
		t.Fatalf("waiting duplicate not promoted: %v", tk)
	}
	if s.LoadDiagnostics().HasIssues() {
		t.Errorf("diagnostics not cleared: %v", s.LoadDiagnostics().Rejections())
	}
}

func TestReloadPaths_IgnoredAndHiddenPaths(t *testing.T) {
	root := t.TempDir()
	writeRawFile(t, filepath.Join(root, ".tikiignore"), "ignored/\n")
	s, _, _ := newReloadStore(t, root)

	ignored := filepath.Join(root, "ignored", "x.md")
	hidden := filepath.Join(root, ".hidden", "y.md")
	legacy := filepath.Join(root, ".doc", "z.md")
	writeExternal(t, ignored, docContent("IGNORE", "ignored"))
	writeExternal(t, hidden, docContent("HIDDEN", "hidden"))
	writeExternal(t, legacy, docContent("LEGACY", "legacy"))

	res := mustReload(t, s, ignored, hidden, legacy)

	if got := sortedIDs(res.Added); len(got) != 1 || got[0] != "LEGACY" {
		t.Errorf("Added = %v, want [LEGACY]", got)
	}
}

func TestReloadPaths_KnownFileBecomesIgnored(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.md")
	writeDoc(t, path, "AAAAAA", "a")
	s, _, _ := newReloadStore(t, root)

	writeRawFile(t, filepath.Join(root, ".tikiignore"), "a.md\n")
	mustReload(t, s, path)

	if s.GetTiki("AAAAAA") != nil {
		t.Error("tiki in a newly ignored file is still loaded")
	}
}

func TestReloadPaths_IDLessFileIsNoop(t *testing.T) {
	root := t.TempDir()
	s, _, notified := newReloadStore(t, root)

	readme := filepath.Join(root, "README.md")
	writeExternal(t, readme, "# just docs\n")
	res := mustReload(t, s, readme)

	if res.Changed || *notified != 0 {
		t.Errorf("id-less file changed the store: %+v, notified %d", res, *notified)
	}
}

func TestReloadPaths_KnownTikiLosesID(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.md")
	writeDoc(t, path, "AAAAAA", "a")
	s, _, _ := newReloadStore(t, root)

	writeExternal(t, path, "---\ntitle: no id now\n---\nbody\n")
	mustReload(t, s, path)

	if s.GetTiki("AAAAAA") != nil {
		t.Error("tiki whose file lost its id is still loaded")
	}
	if s.LoadDiagnostics().HasIssues() {
		t.Errorf("an id-less file must not be a diagnostic: %v", s.LoadDiagnostics().Rejections())
	}
}

func TestReloadPaths_EmptyFileDeferredOnce(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.md")
	writeDoc(t, path, "AAAAAA", "a")
	s, _, notified := newReloadStore(t, root)
	before := s.GetTiki("AAAAAA")

	writeExternal(t, path, "")
	res := mustReload(t, s, path)

	if len(res.Retry) != 1 || res.Retry[0] != path {
		t.Errorf("Retry = %v, want [%s]", res.Retry, path)
	}
	if res.Changed || *notified != 0 || s.GetTiki("AAAAAA") != before {
		t.Fatalf("empty file (likely mid-write) changed the store on first sight: %+v", res)
	}

	res = mustReload(t, s, path)
	if len(res.Retry) != 0 {
		t.Errorf("deferred twice: Retry = %v", res.Retry)
	}
	if s.GetTiki("AAAAAA") != nil {
		t.Error("still-empty file on retry should drop the tiki")
	}
}

func TestReloadPaths_ParseErrorDeferredThenRecordedThenFixed(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.md")
	writeDoc(t, path, "AAAAAA", "a")
	s, _, _ := newReloadStore(t, root)

	broken := "---\nid: AAAAAA\ntitle: [unclosed\n---\nbody\n"
	writeExternal(t, path, broken)
	res := mustReload(t, s, path)
	if len(res.Retry) != 1 || s.GetTiki("AAAAAA") == nil {
		t.Fatalf("parse error on first sight should defer: %+v", res)
	}

	mustReload(t, s, path)
	if s.GetTiki("AAAAAA") != nil {
		t.Error("persistently broken file should drop the tiki")
	}
	rejections := s.LoadDiagnostics().Rejections()
	if len(rejections) != 1 || rejections[0].Reason != LoadReasonParseError {
		t.Fatalf("rejections = %+v, want one parse error", rejections)
	}

	writeExternal(t, path, docContent("AAAAAA", "fixed"))
	mustReload(t, s, path)
	if tk := s.GetTiki("AAAAAA"); tk == nil || tk.Title() != "fixed" {
		t.Errorf("fixed file not reloaded: %v", tk)
	}
	if s.LoadDiagnostics().HasIssues() {
		t.Errorf("diagnostics not cleared after fix: %v", s.LoadDiagnostics().Rejections())
	}
}

func TestReloadPaths_BatchNotifiesOnce(t *testing.T) {
	root := t.TempDir()
	pathA := filepath.Join(root, "a.md")
	pathB := filepath.Join(root, "b.md")
	writeDoc(t, pathA, "AAAAAA", "a")
	writeDoc(t, pathB, "BBBBBB", "b")
	s, _, notified := newReloadStore(t, root)

	pathC := filepath.Join(root, "c.md")
	writeExternal(t, pathA, docContent("AAAAAA", "a2"))
	if err := os.Remove(pathB); err != nil {
		t.Fatal(err)
	}
	writeExternal(t, pathC, docContent("CCCCCC", "c"))

	mustReload(t, s, pathA, pathB, pathC)
	if *notified != 1 {
		t.Errorf("notified %d times for one batch, want 1", *notified)
	}
}

func TestReloadPaths_NoGitHistoryCalls(t *testing.T) {
	root := t.TempDir()
	pathA := filepath.Join(root, "a.md")
	pathB := filepath.Join(root, "b.md")
	writeDoc(t, pathA, "AAAAAA", "a")
	writeDoc(t, pathB, "BBBBBB", "b")
	s, g, _ := newReloadStore(t, root)

	writeExternal(t, pathA, docContent("AAAAAA", "a2"))
	writeExternal(t, filepath.Join(root, "c.md"), docContent("CCCCCC", "c"))
	mustReload(t, s, pathA, filepath.Join(root, "c.md"))
	// id change forces the git-free reconcile path too
	writeExternal(t, pathB, docContent("DDDDDD", "b"))
	mustReload(t, s, pathB)
	if _, err := s.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if n := g.historyCalls(); n != 0 {
		t.Errorf("incremental reload issued %d git history calls, want 0", n)
	}
}

func TestReloadPaths_ParityWithFreshLoad(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, filepath.Join(root, "a.md"), "AAAAAA", "a")
	writeDoc(t, filepath.Join(root, "b.md"), "BBBBBB", "b")
	writeDoc(t, filepath.Join(root, "c.md"), "CCCCCC", "c")
	writeDoc(t, filepath.Join(root, "d.md"), "DDDDDD", "d")
	s, _, _ := newReloadStore(t, root)

	p := func(rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }

	writeExternal(t, p("nested/e.md"), docContent("EEEEEE", "e"))
	mustReload(t, s, p("nested/e.md"))

	writeExternal(t, p("a.md"), docContent("AAAAAA", "a edited"))
	mustReload(t, s, p("a.md"))

	if err := os.Remove(p("b.md")); err != nil {
		t.Fatal(err)
	}
	mustReload(t, s, p("b.md"))

	if err := os.Rename(p("c.md"), p("nested/c-renamed.md")); err != nil {
		t.Fatal(err)
	}
	mustReload(t, s, p("c.md"), p("nested/c-renamed.md"))

	writeExternal(t, p("d.md"), docContent("FFFFFF", "d with new id"))
	mustReload(t, s, p("d.md"))

	writeExternal(t, p("z-dup.md"), docContent("AAAAAA", "duplicate of a"))
	mustReload(t, s, p("z-dup.md"))

	fresh, err := NewTikiStore(root)
	if err != nil {
		t.Fatalf("fresh NewTikiStore: %v", err)
	}
	assertSameDocuments(t, s, fresh)
}

func TestReconcile_PicksUpExternalChanges(t *testing.T) {
	root := t.TempDir()
	pathA := filepath.Join(root, "a.md")
	pathB := filepath.Join(root, "b.md")
	writeDoc(t, pathA, "AAAAAA", "a")
	writeDoc(t, pathB, "BBBBBB", "b")
	writeDoc(t, filepath.Join(root, "keep.md"), "KEEPME", "keep")
	s, g, notified := newReloadStore(t, root)
	keep := s.GetTiki("KEEPME")

	writeExternal(t, pathA, docContent("AAAAAA", "a2"))
	if err := os.Remove(pathB); err != nil {
		t.Fatal(err)
	}
	writeExternal(t, filepath.Join(root, "sub", "c.md"), docContent("CCCCCC", "c"))

	res, err := s.Reconcile()
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got := sortedIDs(res.Added); len(got) != 1 || got[0] != "CCCCCC" {
		t.Errorf("Added = %v, want [CCCCCC]", got)
	}
	if got := sortedIDs(res.Removed); len(got) != 1 || got[0] != "BBBBBB" {
		t.Errorf("Removed = %v, want [BBBBBB]", got)
	}
	if got := sortedIDs(res.Updated); len(got) != 1 || got[0] != "AAAAAA" {
		t.Errorf("Updated = %v, want [AAAAAA]", got)
	}
	if s.GetTiki("KEEPME") != keep {
		t.Error("unchanged tiki was replaced by Reconcile")
	}
	if *notified != 1 {
		t.Errorf("notified %d times, want 1", *notified)
	}
	if n := g.historyCalls(); n != 0 {
		t.Errorf("Reconcile issued %d git history calls, want 0", n)
	}

	*notified = 0
	res, err = s.Reconcile()
	if err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}
	if res.Changed || *notified != 0 {
		t.Errorf("no-op Reconcile reported changes: %+v, notified %d", res, *notified)
	}
}

func TestExternallyChanged(t *testing.T) {
	root := t.TempDir()
	selfPath := filepath.Join(root, "self.md")
	extPath := filepath.Join(root, "ext.md")
	gonePath := filepath.Join(root, "gone.md")
	writeDoc(t, selfPath, "SELFED", "self")
	writeDoc(t, extPath, "EXTERN", "ext")
	writeDoc(t, gonePath, "GONEGO", "gone")
	s, _, _ := newReloadStore(t, root)

	updated := s.GetTiki("SELFED").Clone()
	updated.SetTitle("self edited")
	if err := s.UpdateTiki(updated); err != nil {
		t.Fatalf("UpdateTiki: %v", err)
	}
	created := tikipkg.New()
	created.SetTitle("created in app")
	if err := s.CreateTiki(created); err != nil {
		t.Fatalf("CreateTiki: %v", err)
	}
	appDeletedPath := s.PathForID("GONEGO")
	s.DeleteTiki("GONEGO")

	writeExternal(t, extPath, docContent("EXTERN", "edited elsewhere"))
	newPath := filepath.Join(root, "brand-new.md")
	writeExternal(t, newPath, docContent("BRANDN", "new"))

	missingUnknown := filepath.Join(root, "never-existed.md")

	got := s.ExternallyChanged([]string{selfPath, created.Path(), appDeletedPath, extPath, newPath, missingUnknown})
	sort.Strings(got)
	want := []string{newPath, extPath}
	sort.Strings(want)
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("ExternallyChanged = %v, want %v", got, want)
	}

	if err := os.Remove(extPath); err != nil {
		t.Fatal(err)
	}
	if got := s.ExternallyChanged([]string{extPath}); len(got) != 1 {
		t.Errorf("externally deleted known file not reported: %v", got)
	}
}

func TestLoadedSizeTracksFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.md")
	writeDoc(t, path, "AAAAAA", "a")
	s, _, _ := newReloadStore(t, root)

	assertSize := func(stage string) {
		t.Helper()
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := s.GetTiki("AAAAAA").LoadedSize; got != info.Size() {
			t.Errorf("%s: LoadedSize = %d, want %d", stage, got, info.Size())
		}
	}
	assertSize("after load")

	updated := s.GetTiki("AAAAAA").Clone()
	updated.SetTitle("a much longer title than before")
	if err := s.UpdateTiki(updated); err != nil {
		t.Fatalf("UpdateTiki: %v", err)
	}
	assertSize("after save")

	if c := s.GetTiki("AAAAAA").Clone(); c.LoadedSize != s.GetTiki("AAAAAA").LoadedSize {
		t.Error("Clone dropped LoadedSize")
	}
}

func assertSameDocuments(t *testing.T, got, want *TikiStore) {
	t.Helper()
	g, w := got.GetAllTikis(), want.GetAllTikis()
	if len(g) != len(w) {
		t.Fatalf("document count = %d, fresh load has %d\n got:  %v\n want: %v", len(g), len(w), docSummary(g), docSummary(w))
	}
	for i := range w {
		if g[i].ID() != w[i].ID() || g[i].Path() != w[i].Path() || g[i].Title() != w[i].Title() || g[i].Body() != w[i].Body() {
			t.Errorf("document %d differs:\n got:  %s %s %q\n want: %s %s %q",
				i, g[i].ID(), g[i].Path(), g[i].Title(), w[i].ID(), w[i].Path(), w[i].Title())
		}
	}
	gr, wr := got.LoadDiagnostics().Rejections(), want.LoadDiagnostics().Rejections()
	if len(gr) != len(wr) {
		t.Fatalf("rejections differ:\n got:  %+v\n want: %+v", gr, wr)
	}
	for i := range wr {
		if gr[i].Path != wr[i].Path || gr[i].Reason != wr[i].Reason {
			t.Errorf("rejection %d differs: got %+v, want %+v", i, gr[i], wr[i])
		}
	}
}

func docSummary(tks []*tikipkg.Tiki) []string {
	out := make([]string, 0, len(tks))
	for _, tk := range tks {
		out = append(out, tk.ID()+"@"+filepath.Base(tk.Path()))
	}
	return out
}

func writeRawFile(t *testing.T, path, content string) {
	t.Helper()
	//nolint:gosec // G306: test temp file
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
