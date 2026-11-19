package tikistore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boolean-maybe/tiki/config"
	"github.com/boolean-maybe/tiki/internal/idseq"
	"github.com/boolean-maybe/tiki/internal/teststatuses"
	tikipkg "github.com/boolean-maybe/tiki/tiki"
	"github.com/boolean-maybe/tiki/workflow"
)

type memAllocator struct{ counters map[string]int }

func newMemAllocator() *memAllocator { return &memAllocator{counters: map[string]int{}} }

func (m *memAllocator) Next(prefix string, floor int) (int, error) {
	n := max(m.counters[prefix], floor) + 1
	m.counters[prefix] = n
	return n, nil
}

func (m *memAllocator) Raise(prefix string, atLeast int) error {
	m.counters[prefix] = max(m.counters[prefix], atLeast)
	return nil
}

func (m *memAllocator) Peek(prefix string) (int, error) { return m.counters[prefix], nil }

// usePrefixedWorkflow registers the canonical test fields plus area/layer prefix fields.
func usePrefixedWorkflow(t *testing.T) {
	t.Helper()
	extra := []workflow.FieldDef{
		{
			Name: "area", Type: workflow.TypeEnum, Required: true, Immutable: true,
			EnumValues: []workflow.EnumValue{{Value: "feature", Prefix: "FEAT"}, {Value: "misc", Default: true}},
		},
		{
			Name: "layer", Type: workflow.TypeEnum, Required: true, Immutable: true,
			EnumValues: []workflow.EnumValue{{Value: "frontend", Prefix: "FE"}, {Value: "cross", Default: true}},
		},
	}
	if err := teststatuses.InitWith(extra); err != nil {
		t.Fatal(err)
	}
	workflow.SetProjectPrefix("TIK")
	t.Cleanup(func() {
		workflow.ClearWorkflowFields()
		teststatuses.Init()
	})
}

func createTitled(t *testing.T, s *TikiStore, title string, fields map[string]string) *tikipkg.Tiki {
	t.Helper()
	tk, err := s.NewTikiTemplate()
	if err != nil {
		t.Fatal(err)
	}
	tk.SetTitle(title)
	for k, v := range fields {
		tk.Set(k, v)
	}
	if err := s.CreateTiki(tk); err != nil {
		t.Fatalf("CreateTiki(%s): %v", title, err)
	}
	return tk
}

func TestPrefixedCreateUsesProjectPrefix(t *testing.T) {
	usePrefixedWorkflow(t)
	s, err := NewTikiStore(t.TempDir(), WithIDAllocator(newMemAllocator()))
	if err != nil {
		t.Fatal(err)
	}
	if got := createTitled(t, s, "one", nil).ID(); got != "TIK-1" {
		t.Errorf("first id = %q, want TIK-1", got)
	}
	if got := createTitled(t, s, "two", nil).ID(); got != "TIK-2" {
		t.Errorf("second id = %q, want TIK-2", got)
	}
}

func TestPrefixedCreateComposesFieldPrefixes(t *testing.T) {
	usePrefixedWorkflow(t)
	s, err := NewTikiStore(t.TempDir(), WithIDAllocator(newMemAllocator()))
	if err != nil {
		t.Fatal(err)
	}
	if got := createTitled(t, s, "a", map[string]string{"area": "feature", "layer": "frontend"}).ID(); got != "FEAT-FE-1" {
		t.Errorf("id = %q, want FEAT-FE-1", got)
	}
	if got := createTitled(t, s, "b", map[string]string{"area": "feature", "layer": "cross"}).ID(); got != "FEAT-1" {
		t.Errorf("id = %q, want FEAT-1", got)
	}
}

func TestProvisionalTemplateIDIsReplaced(t *testing.T) {
	usePrefixedWorkflow(t)
	s, err := NewTikiStore(t.TempDir(), WithIDAllocator(newMemAllocator()))
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := s.NewTikiTemplate()
	if err != nil {
		t.Fatal(err)
	}
	provisional := tmpl.ID()
	if !tmpl.ProvisionalID() {
		t.Fatal("a template under a prefixed workflow must carry a provisional id")
	}
	if !tmpl.Clone().ProvisionalID() {
		t.Fatal("Clone must keep the provisional marker")
	}
	tmpl.SetTitle("draft")
	if err := s.CreateTiki(tmpl); err != nil {
		t.Fatal(err)
	}
	if tmpl.ID() != "TIK-1" {
		t.Errorf("saved id = %q, want TIK-1", tmpl.ID())
	}
	if tmpl.ProvisionalID() {
		t.Error("a saved tiki must not stay provisional")
	}
	// views opened on the draft still hold the provisional id
	if got := s.GetTiki(provisional); got == nil || got.ID() != "TIK-1" {
		t.Errorf("GetTiki(provisional) = %v, want the saved TIK-1", got)
	}
	if n := len(s.GetAllTikis()); n != 1 {
		t.Errorf("GetAllTikis has %d tikis, want 1 (no alias duplicates)", n)
	}
	s.DeleteTiki(provisional)
	if s.GetTiki("TIK-1") != nil {
		t.Error("DeleteTiki(provisional) left TIK-1 in the store")
	}
}

func TestPrefixedFloorComesFromLoadedIDs(t *testing.T) {
	usePrefixedWorkflow(t)
	dir := t.TempDir()
	files := map[string]string{
		"seven.md":  "---\nid: TIK-7\ntitle: seven\n---\n",
		"former.md": "---\nid: ABC123\ntitle: former\nformerIds:\n  - TIK-12\n---\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	alloc := newMemAllocator()
	alloc.counters["TIK"] = 2
	s, err := NewTikiStore(dir, WithIDAllocator(alloc))
	if err != nil {
		t.Fatal(err)
	}
	if got := createTitled(t, s, "next", nil).ID(); got != "TIK-13" {
		t.Errorf("id = %q, want TIK-13 (above loaded TIK-7 and former TIK-12)", got)
	}
}

func TestWithoutPrefixTemplateIDIsKept(t *testing.T) {
	s, err := NewTikiStore(t.TempDir(), WithIDAllocator(newMemAllocator()))
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := s.NewTikiTemplate()
	if err != nil {
		t.Fatal(err)
	}
	if tmpl.ProvisionalID() {
		t.Fatal("no prefix configured, yet the template id is provisional")
	}
	want := tmpl.ID()
	tmpl.SetTitle("plain")
	if err := s.CreateTiki(tmpl); err != nil {
		t.Fatal(err)
	}
	if tmpl.ID() != want || len(want) != 6 {
		t.Errorf("id = %q, want the template's random id %q", tmpl.ID(), want)
	}
}

func TestDefaultAllocatorFileIsKeyedByRootCommit(t *testing.T) {
	usePrefixedWorkflow(t)
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "root")
	root := strings.TrimSpace(runGit(t, repo, "rev-list", "--max-parents=0", "HEAD"))
	t.Chdir(repo)
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	config.ResetPathManager()
	t.Cleanup(config.ResetPathManager)

	s, err := NewTikiStore(repo)
	if err != nil {
		t.Fatal(err)
	}
	if got := createTitled(t, s, "one", nil).ID(); got != "TIK-1" {
		t.Fatalf("id = %q, want TIK-1", got)
	}
	want := filepath.Join(state, "tiki", "sequences", idseq.FileName(filepath.Base(repo), "commit:"+root))
	data, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("counter file %s: %v", want, err)
	}
	if !strings.Contains(string(data), "TIK: 1") {
		t.Errorf("counter file lacks TIK: 1:\n%s", data)
	}
}

func TestDefaultAllocatorRefusesAShallowClone(t *testing.T) {
	usePrefixedWorkflow(t)
	origin := t.TempDir()
	runGit(t, origin, "init", "-q")
	for _, msg := range []string{"one", "two"} {
		runGit(t, origin, "-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", msg)
	}
	shallow := filepath.Join(t.TempDir(), "shallow")
	runGit(t, origin, "clone", "-q", "--depth", "1", "file://"+origin, shallow)
	t.Chdir(shallow)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	config.ResetPathManager()
	t.Cleanup(config.ResetPathManager)

	s, err := NewTikiStore(shallow)
	if err != nil {
		t.Fatal(err)
	}
	tk, err := s.NewTikiTemplate()
	if err != nil {
		t.Fatal(err)
	}
	tk.SetTitle("in a shallow clone")
	if err := s.CreateTiki(tk); err == nil || !strings.Contains(err.Error(), "shallow") {
		t.Fatalf("create in a shallow clone = %v, want a shallow-clone error", err)
	}
}

func TestDefaultAllocatorKeyFallbacks(t *testing.T) {
	usePrefixedWorkflow(t)
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)
		want  func(dir string) string
	}{
		{"repository without commits", func(t *testing.T, dir string) { runGit(t, dir, "init", "-q") },
			func(dir string) string {
				return idseq.FileName(filepath.Base(dir), "gitdir:"+filepath.Join(dir, ".git"))
			}},
		{"no git", func(*testing.T, string) {},
			func(dir string) string { return idseq.FileName(filepath.Base(dir), "dir:"+dir) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			tt.setup(t, dir)
			t.Chdir(dir)
			state := t.TempDir()
			t.Setenv("XDG_STATE_HOME", state)
			config.ResetPathManager()
			t.Cleanup(config.ResetPathManager)
			s, err := NewTikiStore(dir)
			if err != nil {
				t.Fatal(err)
			}
			createTitled(t, s, "one", nil)
			if _, err := os.Stat(filepath.Join(state, "tiki", "sequences", tt.want(dir))); err != nil {
				entries, _ := os.ReadDir(filepath.Join(state, "tiki", "sequences"))
				t.Errorf("counter file %s missing (%v); sequences dir holds %v", tt.want(dir), err, entries)
			}
		})
	}
}

func TestFailedCreateKeepsItsProvisionalID(t *testing.T) {
	usePrefixedWorkflow(t)
	s, err := NewTikiStore(t.TempDir(), WithIDAllocator(newMemAllocator()))
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := s.NewTikiTemplate()
	if err != nil {
		t.Fatal(err)
	}
	placeholder := tmpl.ID()
	tmpl.SetTitle("draft")
	tmpl.Set("area", "feature")
	tmpl.Set("dependsOn", []string{"ZZZZZZ"})
	if err := s.CreateTiki(tmpl); err == nil {
		t.Fatal("create with a dangling dependency succeeded")
	}
	if tmpl.ID() != placeholder || !tmpl.ProvisionalID() {
		t.Fatalf("after a failed create: id %q provisional %v, want %q and provisional", tmpl.ID(), tmpl.ProvisionalID(), placeholder)
	}
	if s.GetTiki(placeholder) != nil {
		t.Error("a failed create left an alias for its placeholder")
	}
	tmpl.Set("area", "misc")
	tmpl.Set("dependsOn", []string{})
	if err := s.CreateTiki(tmpl); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tmpl.ID(), "TIK-") {
		t.Errorf("retried create got %q; its id must follow the fields it has now", tmpl.ID())
	}
}

func TestAllocationRefusesAnIDTheLoaderWouldReject(t *testing.T) {
	usePrefixedWorkflow(t)
	alloc := newMemAllocator()
	alloc.counters["TIK"] = 999999999
	s, err := NewTikiStore(t.TempDir(), WithIDAllocator(alloc))
	if err != nil {
		t.Fatal(err)
	}
	tk, err := s.NewTikiTemplate()
	if err != nil {
		t.Fatal(err)
	}
	tk.SetTitle("one too many")
	if err := s.CreateTiki(tk); err == nil || !strings.Contains(err.Error(), "TIK-1000000000") {
		t.Fatalf("create past the number cap = %v, want an error naming TIK-1000000000", err)
	}
}

func TestCounterFileIsLabelledAfterTheMainCheckout(t *testing.T) {
	usePrefixedWorkflow(t)
	base := t.TempDir()
	repo := filepath.Join(base, "myproject")
	if err := os.Mkdir(repo, 0o750); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "root")
	root := strings.TrimSpace(runGit(t, repo, "rev-list", "--max-parents=0", "HEAD"))
	wt := filepath.Join(base, "side-work")
	runGit(t, repo, "worktree", "add", "-q", "-b", "side", wt)
	t.Chdir(wt)
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	config.ResetPathManager()
	t.Cleanup(config.ResetPathManager)

	s, err := NewTikiStore(wt)
	if err != nil {
		t.Fatal(err)
	}
	createTitled(t, s, "from the worktree", nil)
	want := filepath.Join(state, "tiki", "sequences", idseq.FileName("myproject", "commit:"+root))
	path, counters, exists, err := s.CounterFile()
	if err != nil || !exists || path != want || counters["TIK"] != 1 {
		t.Errorf("CounterFile = %q, %v, %v, %v; want %q holding TIK: 1", path, counters, exists, err, want)
	}
}
