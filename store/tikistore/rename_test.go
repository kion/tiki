package tikistore

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/boolean-maybe/tiki/internal/livestatus"
)

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func renameFixture(t *testing.T) (string, *TikiStore, *memAllocator) {
	t.Helper()
	usePrefixedWorkflow(t)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"a.md":          "---\nid: X7F4K2\ntitle: a\narea: feature\nlayer: cross\n---\nsee [[QQQQQQ]], [q](QQQQQQ), [QQQQQQ](QQQQQQ) and plain QQQQQQ\n",
		"b.md":          "---\nid: QQQQQQ\ntitle: b\ncreatedAt: 2024-01-02T03:04:05Z\n---\nbody\n",
		"c.md":          "---\nid: CCCCCC\ntitle: c\ndependsOn:\n  - QQQQQQ\n---\n",
		"notes/wiki.md": "# Notes\n\nlink [[QQQQQQ]] and [x](qqqqqq) and QQQQQQ\n",
	})
	if err := livestatus.Write(dir, "QQQQQQ", livestatus.Status{Message: "busy"}); err != nil {
		t.Fatal(err)
	}
	alloc := newMemAllocator()
	s, err := NewTikiStore(dir, WithIDAllocator(alloc))
	if err != nil {
		t.Fatal(err)
	}
	return dir, s, alloc
}

func TestApplyRenamesRewritesEveryReference(t *testing.T) {
	dir, s, alloc := renameFixture(t)
	changed, err := s.ApplyRenames([]Rename{{Old: "QQQQQQ", New: "TIK-5"}}, false)
	if err != nil {
		t.Fatalf("ApplyRenames: %v", err)
	}

	b := readFile(t, filepath.Join(dir, "b.md"))
	for _, want := range []string{"id: TIK-5", "createdAt: 2024-01-02T03:04:05Z", "formerIds:\n    - QQQQQQ"} {
		if !strings.Contains(b, want) {
			t.Errorf("b.md lacks %q:\n%s", want, b)
		}
	}
	a := readFile(t, filepath.Join(dir, "a.md"))
	if !strings.Contains(a, "see [[TIK-5]], [q](TIK-5), [TIK-5](TIK-5) and plain QQQQQQ") {
		t.Errorf("a.md links not rewritten (plain text must stay):\n%s", a)
	}
	if c := readFile(t, filepath.Join(dir, "c.md")); !strings.Contains(c, "- TIK-5") || strings.Contains(c, "QQQQQQ") {
		t.Errorf("c.md dependsOn not rewritten:\n%s", c)
	}
	if w := readFile(t, filepath.Join(dir, "notes", "wiki.md")); !strings.Contains(w, "link [[TIK-5]] and [x](TIK-5) and QQQQQQ") {
		t.Errorf("wiki page links not rewritten:\n%s", w)
	}

	if s.GetTiki("TIK-5") == nil || s.GetTiki("QQQQQQ") != nil {
		t.Error("the store still knows the tiki by its old id")
	}
	if got := s.GetTiki("TIK-5").CreatedAt(); !got.Equal(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Errorf("createdAt changed to %v", got)
	}
	if _, ok, _ := livestatus.Read(dir, "TIK-5"); !ok {
		t.Error("live status did not follow the rename")
	}
	if _, ok, _ := livestatus.Read(dir, "QQQQQQ"); ok {
		t.Error("live status for the old id is still there")
	}
	if n, _ := alloc.Peek("TIK"); n != 5 {
		t.Errorf("counter for TIK = %d, want raised to 5", n)
	}
	for _, want := range []string{"a.md", "b.md", "c.md", filepath.Join("notes", "wiki.md")} {
		if !slices.Contains(changed, filepath.Join(dir, want)) {
			t.Errorf("changed files %v lack %s", changed, want)
		}
	}
}

func TestApplyRenamesDryRunWritesNothing(t *testing.T) {
	dir, s, alloc := renameFixture(t)
	before := readFile(t, filepath.Join(dir, "a.md"))
	changed, err := s.ApplyRenames([]Rename{{Old: "QQQQQQ", New: "TIK-5"}}, true)
	if err != nil {
		t.Fatalf("ApplyRenames dry run: %v", err)
	}
	if len(changed) != 4 {
		t.Errorf("dry run reports %d files, want 4: %v", len(changed), changed)
	}
	if readFile(t, filepath.Join(dir, "a.md")) != before {
		t.Error("dry run changed a.md")
	}
	if s.GetTiki("QQQQQQ") == nil {
		t.Error("dry run renamed the tiki in the store")
	}
	if n, _ := alloc.Peek("TIK"); n != 0 {
		t.Errorf("dry run raised the counter to %d", n)
	}
}

func TestCheckRenameRefusals(t *testing.T) {
	_, s, _ := renameFixture(t)
	if _, err := s.ApplyRenames([]Rename{{Old: "QQQQQQ", New: "TIK-5"}}, false); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		old, new, want string
	}{
		{"ZZZZZZ", "TIK-9", "no tiki with id ZZZZZZ"},
		{"CCCCCC", "TIK-05", "not a valid id"},
		{"CCCCCC", "X7F4K2", "already taken"},
		{"CCCCCC", "QQQQQQ", "former id of TIK-5"},
		{"X7F4K2", "TIK-9", "fields give the prefix FEAT"},
	}
	for _, tt := range tests {
		err := s.CheckRename(tt.old, tt.new)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("CheckRename(%s, %s) = %v, want %q", tt.old, tt.new, err, tt.want)
		}
	}
	if err := s.CheckRename("X7F4K2", "FEAT-3"); err != nil {
		t.Errorf("a matching prefix must pass: %v", err)
	}
}

func TestPlanMigrationOldestFirst(t *testing.T) {
	usePrefixedWorkflow(t)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"new.md":  "---\nid: NNNNNN\ntitle: new\ncreatedAt: 2025-05-05T00:00:00Z\narea: misc\nlayer: cross\n---\n",
		"old.md":  "---\nid: OOOOOO\ntitle: old\ncreatedAt: 2024-04-04T00:00:00Z\ntags:\n  - x\n---\n",
		"note.md": "---\nid: PPPPPP\ntitle: notes only\ncreatedAt: 2024-06-06T00:00:00Z\n---\n",
		"feat.md": "---\nid: FFFFFF\ntitle: feat\ncreatedAt: 2023-03-03T00:00:00Z\narea: feature\nlayer: frontend\n---\n",
		"done.md": "---\nid: TIK-7\ntitle: already prefixed\n---\n",
	})
	s, err := NewTikiStore(dir, WithIDAllocator(newMemAllocator()))
	if err != nil {
		t.Fatal(err)
	}
	renames, empty, err := s.PlanMigration()
	if err != nil {
		t.Fatalf("PlanMigration: %v", err)
	}
	want := []Rename{{Old: "FFFFFF", New: "FEAT-FE-1"}, {Old: "OOOOOO", New: "TIK-8"}, {Old: "PPPPPP", New: "TIK-9"}, {Old: "NNNNNN", New: "TIK-10"}}
	if !slices.Equal(renames, want) {
		t.Errorf("renames = %v, want %v", renames, want)
	}
	// notes-only PPPPPP has no workflow fields at all, so it isn't listed
	if len(empty) != 1 || empty[0].ID != "OOOOOO" || !slices.Equal(empty[0].Fields, []string{"area", "layer"}) {
		t.Errorf("empty prefix fields = %+v, want OOOOOO: area, layer", empty)
	}
}

func TestPlanMigrationNeedsAPrefix(t *testing.T) {
	s, err := NewTikiStore(t.TempDir(), WithIDAllocator(newMemAllocator()))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.PlanMigration(); err == nil || !strings.Contains(err.Error(), "top-level prefix") {
		t.Fatalf("PlanMigration without a prefix = %v, want an error naming the top-level prefix", err)
	}
}

func TestRenameBackToOwnFormerIDUndoes(t *testing.T) {
	dir, s, _ := renameFixture(t)
	if _, err := s.ApplyRenames([]Rename{{Old: "QQQQQQ", New: "TIK-5"}}, false); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckRename("TIK-5", "QQQQQQ"); err != nil {
		t.Fatalf("renaming back to its own former id must be allowed: %v", err)
	}
	if _, err := s.ApplyRenames([]Rename{{Old: "TIK-5", New: "QQQQQQ"}}, false); err != nil {
		t.Fatal(err)
	}
	b := readFile(t, filepath.Join(dir, "b.md"))
	if !strings.Contains(b, "id: QQQQQQ") || !strings.Contains(b, "- TIK-5") || strings.Contains(b, "- QQQQQQ") {
		t.Errorf("after undo, b.md must have id QQQQQQ and formerIds [TIK-5] only:\n%s", b)
	}
}

func TestApplyRenamesFailsWithoutWritingAnything(t *testing.T) {
	dir, s, alloc := renameFixture(t)
	before := map[string]string{}
	for _, name := range []string{"a.md", "b.md", "c.md", filepath.Join("notes", "wiki.md")} {
		before[name] = readFile(t, filepath.Join(dir, name))
	}
	locked := filepath.Join(dir, "c.md")
	//nolint:gosec // G302: a read-only file is what this test needs
	if err := os.Chmod(locked, 0o444); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // G302: restores the permission the fixture wrote
	t.Cleanup(func() { _ = os.Chmod(locked, 0o644) })

	if _, err := s.ApplyRenames([]Rename{{Old: "QQQQQQ", New: "TIK-5"}}, false); err == nil {
		t.Fatal("ApplyRenames succeeded although c.md is read-only")
	}
	for name, content := range before {
		if got := readFile(t, filepath.Join(dir, name)); got != content {
			t.Errorf("%s changed although the rename failed:\n%s", name, got)
		}
	}
	if s.GetTiki("QQQQQQ") == nil {
		t.Error("the store renamed the tiki although the rename failed")
	}
	if n, _ := alloc.Peek("TIK"); n != 0 {
		t.Errorf("counter raised to %d by a failed rename", n)
	}
}

func TestApplyRenamesHealsReferencesToFormerIDs(t *testing.T) {
	usePrefixedWorkflow(t)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"moved.md": "---\nid: TIK-1\ntitle: moved\nformerIds:\n  - AAAAA1\n---\n",
		"stale.md": "---\nid: BBBBBB\ntitle: stale\ndependsOn:\n  - AAAAA1\n---\nsee [[AAAAA1]]\n",
		"page.md":  "# page\n\n[old](AAAAA1)\n",
	})
	s, err := NewTikiStore(dir, WithIDAllocator(newMemAllocator()))
	if err != nil {
		t.Fatal(err)
	}
	changed, err := s.ApplyRenames(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 2 {
		t.Errorf("changed = %v, want stale.md and page.md", changed)
	}
	if st := readFile(t, filepath.Join(dir, "stale.md")); !strings.Contains(st, "- TIK-1") || !strings.Contains(st, "see [[TIK-1]]") {
		t.Errorf("stale.md not healed:\n%s", st)
	}
	if p := readFile(t, filepath.Join(dir, "page.md")); !strings.Contains(p, "[old](TIK-1)") {
		t.Errorf("page.md not healed:\n%s", p)
	}
}

func TestRewriteLinksCoversCommonMarkLinkForms(t *testing.T) {
	mapping := map[string]string{"QQQQQQ": "TIK-5"}
	tests := []struct{ in, want string }{
		{"[alpha\nbeta](QQQQQQ)", "[alpha\nbeta](TIK-5)"},
		{"[see [1] here](QQQQQQ)", "[see [1] here](TIK-5)"},
		{`[alpha](QQQQQQ "title")`, `[alpha](TIK-5 "title")`},
		{"[alpha](<QQQQQQ>)", "[alpha](<TIK-5>)"},
		{"[a]: QQQQQQ", "[a]: TIK-5"},
		{`[a]: <qqqqqq> "t"`, `[a]: <TIK-5> "t"`},
		{"[QQQQQQ](QQQQQQ)", "[TIK-5](TIK-5)"},
		{"[QQQQQQ](<QQQQQQ>)", "[TIK-5](<TIK-5>)"},
		{"plain QQQQQQ and `QQQQQQ`", "plain QQQQQQ and `QQQQQQ`"},
		{"[x](QQQQQQX)", "[x](QQQQQQX)"},
	}
	for _, tt := range tests {
		if got := rewriteLinks(tt.in, mapping); got != tt.want {
			t.Errorf("rewriteLinks(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCheckRenameRefusesATrackedTikiWithAnEmptyPrefixField(t *testing.T) {
	_, s, _ := renameFixture(t)
	// c.md is tracked (dependsOn) but has no area or layer
	err := s.CheckRename("CCCCCC", "TIK-9")
	if err == nil || !strings.Contains(err.Error(), "no value in area, layer") || !strings.Contains(err.Error(), `tiki exec 'update where id = "CCCCCC" set area="feature"'`) {
		t.Fatalf("CheckRename = %v, want a refusal naming area, layer with a fill example", err)
	}
	// b.md is notes-only, so it may take a prefixed id with no prefix fields
	if err := s.CheckRename("QQQQQQ", "TIK-9"); err != nil {
		t.Errorf("notes-only tiki refused: %v", err)
	}
}
