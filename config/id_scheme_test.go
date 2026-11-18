package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boolean-maybe/tiki/workflow"
)

const prefixedWorkflowYAML = `
prefix: TIK
fields:
  - name: area
    type: enum
    required: true
    immutable: true
    values:
      - {value: feature, prefix: "FEAT", label: "Feature wish"}
      - {value: memory, prefix: "MEM", label: "Memory & archive"}
      - {value: misc, label: "Misc"}
  - name: note
    type: text
    immutable: true
`

func TestLoadWorkflowFieldsReadsIDScheme(t *testing.T) {
	cwdDir := setupLoadWorkflowFieldsTest(t)
	if err := os.WriteFile(filepath.Join(cwdDir, "workflow.yaml"), []byte(prefixedWorkflowYAML), 0644); err != nil {
		t.Fatal(err)
	}
	if err := LoadWorkflowFields(); err != nil {
		t.Fatalf("LoadWorkflowFields: %v", err)
	}
	if got := workflow.ProjectPrefix(); got != "TIK" {
		t.Errorf("ProjectPrefix = %q, want TIK", got)
	}
	area, ok := workflow.Field("area")
	if !ok {
		t.Fatal("area not registered")
	}
	if !area.Required || !area.Immutable {
		t.Errorf("area Required=%v Immutable=%v, want both true", area.Required, area.Immutable)
	}
	if ev, _ := area.LookupEnum("feature"); ev.Prefix != "FEAT" {
		t.Errorf("feature prefix = %q, want FEAT", ev.Prefix)
	}
	note, _ := workflow.Field("note")
	if !note.Immutable || note.Required {
		t.Errorf("note Required=%v Immutable=%v, want false/true", note.Required, note.Immutable)
	}
}

func TestLoadWorkflowFieldsWithoutPrefixLeavesIDsRandom(t *testing.T) {
	cwdDir := setupLoadWorkflowFieldsTest(t)
	workflow.SetProjectPrefix("STALE")
	content := "fields:\n  - name: status\n    type: enum\n    values: [open, done]\n"
	if err := os.WriteFile(filepath.Join(cwdDir, "workflow.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := LoadWorkflowFields(); err != nil {
		t.Fatalf("LoadWorkflowFields: %v", err)
	}
	if workflow.PrefixedIDsEnabled() {
		t.Errorf("prefixed ids enabled with no top-level prefix (ProjectPrefix=%q)", workflow.ProjectPrefix())
	}
}

func TestLoadWorkflowFieldsRejectsBadIDScheme(t *testing.T) {
	cwdDir := setupLoadWorkflowFieldsTest(t)
	bad := strings.Replace(prefixedWorkflowYAML, "    immutable: true\n    values:", "    values:", 1)
	path := filepath.Join(cwdDir, "workflow.yaml")
	if err := os.WriteFile(path, []byte(bad), 0644); err != nil {
		t.Fatal(err)
	}
	if err := LoadWorkflowFields(); err == nil || !strings.Contains(err.Error(), "immutable: true") {
		t.Fatalf("LoadWorkflowFields error = %v, want the immutable requirement", err)
	}
	if _, err := LoadWorkflowFieldsFromFile(path); err == nil || !strings.Contains(err.Error(), "immutable: true") {
		t.Fatalf("LoadWorkflowFieldsFromFile error = %v, want the immutable requirement", err)
	}
}

func TestUnknownEnumValueKeyListsPrefix(t *testing.T) {
	cwdDir := setupLoadWorkflowFieldsTest(t)
	content := "fields:\n  - name: status\n    type: enum\n    values:\n      - {value: open, colour: red}\n"
	if err := os.WriteFile(filepath.Join(cwdDir, "workflow.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	err := LoadWorkflowFields()
	if err == nil || !strings.Contains(err.Error(), "prefix") {
		t.Fatalf("error = %v, want the valid keys to include prefix", err)
	}
}
