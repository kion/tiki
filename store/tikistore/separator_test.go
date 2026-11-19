package tikistore

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/boolean-maybe/tiki/internal/teststatuses"
	"github.com/boolean-maybe/tiki/workflow"
)

// useUnseparatedWorkflow is a B/W board numbered without a separator: B102, W67.
func useUnseparatedWorkflow(t *testing.T) {
	t.Helper()
	extra := []workflow.FieldDef{{
		Name: "board", Type: workflow.TypeEnum, Required: true, Immutable: true,
		EnumValues: []workflow.EnumValue{{Value: "backlog", Prefix: "B", Default: true}, {Value: "watch", Prefix: "W"}},
	}}
	if err := teststatuses.InitWith(extra); err != nil {
		t.Fatal(err)
	}
	workflow.SetProjectPrefix("T")
	workflow.SetPrefixSeparator("")
	t.Cleanup(func() {
		workflow.ClearWorkflowFields()
		teststatuses.Init()
	})
}

func TestUnseparatedIDsAllocateAboveLoadedOnes(t *testing.T) {
	useUnseparatedWorkflow(t)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"b203.md": "---\nid: B203\ntitle: B203\nboard: backlog\n---\n",
	})
	s, err := NewTikiStore(dir, WithIDAllocator(newMemAllocator()))
	if err != nil {
		t.Fatal(err)
	}
	if got := createTitled(t, s, "next backlog", map[string]string{"board": "backlog"}).ID(); got != "B204" {
		t.Errorf("id = %q, want B204", got)
	}
	if got := createTitled(t, s, "first watch", map[string]string{"board": "watch"}).ID(); got != "W1" {
		t.Errorf("id = %q, want W1", got)
	}
}

func TestRenameAndMigrateWithoutSeparator(t *testing.T) {
	useUnseparatedWorkflow(t)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"a.md":   "---\nid: 14SIOZ\ntitle: B102 · patcher\nboard: backlog\n---\nsee [[QQQQQQ]]\n",
		"b.md":   "---\nid: QQQQQQ\ntitle: W12 · question\nboard: watch\n---\n",
		"old.md": "---\nid: B203\ntitle: already numbered\nboard: backlog\n---\n",
	})
	s, err := NewTikiStore(dir, WithIDAllocator(newMemAllocator()))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CheckRename("14SIOZ", "B-102"); err == nil || !strings.Contains(err.Error(), "separator") {
		t.Errorf("rename to the hyphen form = %v, want a separator error", err)
	}
	if _, err := s.ApplyRenames([]Rename{{Old: "14SIOZ", New: "B102"}, {Old: "QQQQQQ", New: "W12"}}, false); err != nil {
		t.Fatalf("ApplyRenames: %v", err)
	}
	if a := readFile(t, filepath.Join(dir, "a.md")); !strings.Contains(a, "id: B102") || !strings.Contains(a, "see [[W12]]") {
		t.Errorf("a.md after rename:\n%s", a)
	}
	renames, missing, err := s.PlanMigration()
	if err != nil || len(renames) != 0 || len(missing) != 0 {
		t.Errorf("PlanMigration after the renames = %v, %v, %v; want nothing left (B203 already matches the scheme)", renames, missing, err)
	}
}

func TestMigrateWithoutSeparatorSkipsSchemeIDs(t *testing.T) {
	useUnseparatedWorkflow(t)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"legacy.md": "---\nid: X7F4K2\ntitle: legacy\nboard: watch\n---\n",
		"six.md":    "---\nid: B12345\ntitle: six characters, but a scheme id\nboard: backlog\n---\n",
	})
	s, err := NewTikiStore(dir, WithIDAllocator(newMemAllocator()))
	if err != nil {
		t.Fatal(err)
	}
	renames, _, err := s.PlanMigration()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(renames, []Rename{{Old: "X7F4K2", New: "W1"}}) {
		t.Errorf("renames = %v, want only X7F4K2 -> W1", renames)
	}
}
