package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boolean-maybe/tiki/config"
)

const idsWorkflow = `prefix: TIK
fields:
  - name: status
    type: enum
    values: [open, done]
  - name: area
    type: enum
    required: true
    immutable: true
    values:
      - {value: feature, prefix: FEAT}
      - {value: misc}
`

func setupIDsProject(t *testing.T, workflowYAML string, files map[string]string) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dir := t.TempDir()
	t.Chdir(dir)
	config.ResetPathManager()
	t.Cleanup(config.ResetPathManager)
	t.Cleanup(workflowCleanup)
	if workflowYAML != "" {
		files["workflow.yaml"] = workflowYAML
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runIDsTest(t *testing.T, dir string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runIDsIn(dir, args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func fileText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestIDsMigrateDryRunThenRun(t *testing.T) {
	dir := setupIDsProject(t, idsWorkflow, map[string]string{
		"a.md": "---\nid: XXXXXX\ntitle: a\ncreatedAt: 2024-01-01T00:00:00Z\n---\n",
		"b.md": "---\nid: QQQQQQ\ntitle: b\ncreatedAt: 2025-01-01T00:00:00Z\n---\nsee [[XXXXXX]]\n",
	})
	before := fileText(t, filepath.Join(dir, "b.md"))

	code, out, errOut := runIDsTest(t, dir, "migrate", "--dry-run")
	if code != exitOK {
		t.Fatalf("dry run: exit %d, stderr %q", code, errOut)
	}
	for _, want := range []string{"XXXXXX -> TIK-1", "QQQQQQ -> TIK-2", "would update 2 files:", "  b.md", "dry run: nothing written"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output lacks %q:\n%s", want, out)
		}
	}
	if fileText(t, filepath.Join(dir, "b.md")) != before {
		t.Fatal("dry run wrote b.md")
	}

	code, out, errOut = runIDsTest(t, dir, "migrate")
	if code != exitOK {
		t.Fatalf("migrate: exit %d, stderr %q", code, errOut)
	}
	if !strings.Contains(out, "updated 2 files:") {
		t.Errorf("migrate output:\n%s", out)
	}
	if b := fileText(t, filepath.Join(dir, "b.md")); !strings.Contains(b, "id: TIK-2") || !strings.Contains(b, "see [[TIK-1]]") {
		t.Errorf("b.md after migrate:\n%s", b)
	}

	code, out, _ = runIDsTest(t, dir, "migrate")
	if code != exitOK || !strings.Contains(out, "nothing to migrate") {
		t.Errorf("second migrate: exit %d, output %q", code, out)
	}
}

func TestIDsRename(t *testing.T) {
	dir := setupIDsProject(t, idsWorkflow, map[string]string{
		"a.md": "---\nid: XXXXXX\ntitle: a\narea: feature\n---\n",
	})
	code, out, errOut := runIDsTest(t, dir, "rename", "XXXXXX", "FEAT-55")
	if code != exitOK {
		t.Fatalf("rename: exit %d, stderr %q", code, errOut)
	}
	if !strings.Contains(out, "XXXXXX -> FEAT-55") {
		t.Errorf("rename output:\n%s", out)
	}
	if a := fileText(t, filepath.Join(dir, "a.md")); !strings.Contains(a, "id: FEAT-55") {
		t.Errorf("a.md after rename:\n%s", a)
	}
}

func TestIDsRefusalsAndUsage(t *testing.T) {
	dir := setupIDsProject(t, idsWorkflow, map[string]string{
		"a.md": "---\nid: XXXXXX\ntitle: a\narea: feature\n---\n",
	})
	tests := []struct {
		args     []string
		code     int
		inStderr string
	}{
		{nil, exitUsage, "Usage"},
		{[]string{"rename", "XXXXXX"}, exitUsage, "Usage"},
		{[]string{"bogus"}, exitUsage, "unknown ids command"},
		{[]string{"rename", "ZZZZZZ", "TIK-9"}, exitQueryError, "no tiki with id ZZZZZZ"},
		{[]string{"rename", "XXXXXX", "TIK-9"}, exitQueryError, "fields give the prefix FEAT"},
	}
	for _, tt := range tests {
		code, _, errOut := runIDsTest(t, dir, tt.args...)
		if code != tt.code || !strings.Contains(errOut, tt.inStderr) {
			t.Errorf("ids %v: exit %d stderr %q, want exit %d and %q", tt.args, code, errOut, tt.code, tt.inStderr)
		}
	}
}

func TestIDsMigrateWithoutPrefix(t *testing.T) {
	dir := setupIDsProject(t, "fields:\n  - name: status\n    type: enum\n    values: [open, done]\n", map[string]string{
		"a.md": "---\nid: XXXXXX\ntitle: a\n---\n",
	})
	code, _, errOut := runIDsTest(t, dir, "migrate")
	if code != exitUsage || !strings.Contains(errOut, "top-level prefix") {
		t.Errorf("exit %d stderr %q, want exit %d naming the top-level prefix", code, errOut, exitUsage)
	}
}

func TestIDsHelp(t *testing.T) {
	code, out, _ := runIDsTest(t, t.TempDir(), "--help")
	if code != exitOK || !strings.Contains(out, "tiki ids rename") {
		t.Errorf("--help: exit %d, output %q", code, out)
	}
}

func TestIDsMigrateHealsLeftoverReferences(t *testing.T) {
	dir := setupIDsProject(t, idsWorkflow, map[string]string{
		"moved.md": "---\nid: TIK-1\ntitle: moved\nformerIds:\n  - AAAAA1\n---\n",
		"page.md":  "# page\n\nsee [[AAAAA1]]\n",
	})
	code, out, errOut := runIDsTest(t, dir, "migrate")
	if code != exitOK {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if !strings.Contains(out, "updated 1 files:") || !strings.Contains(fileText(t, filepath.Join(dir, "page.md")), "[[TIK-1]]") {
		t.Errorf("leftover reference not healed; output:\n%s", out)
	}
}

func TestIDsRenamePrintsNormalizedIDs(t *testing.T) {
	dir := setupIDsProject(t, idsWorkflow, map[string]string{
		"a.md": "---\nid: XXXXXX\ntitle: a\narea: feature\n---\n",
	})
	code, out, errOut := runIDsTest(t, dir, "rename", "xxxxxx", "feat-55")
	if code != exitOK || !strings.Contains(out, "XXXXXX -> FEAT-55") {
		t.Errorf("exit %d stdout %q stderr %q, want XXXXXX -> FEAT-55", code, out, errOut)
	}
}

func TestIDsMigrateRefusesEmptyPrefixFields(t *testing.T) {
	dir := setupIDsProject(t, idsWorkflow, map[string]string{
		"a.md":    "---\nid: XXXXXX\ntitle: a\nstatus: open\n---\n",
		"note.md": "---\nid: NNNNNN\ntitle: notes only\n---\n",
	})
	before := fileText(t, filepath.Join(dir, "a.md"))
	for _, args := range [][]string{{"migrate", "--dry-run"}, {"migrate"}} {
		code, _, errOut := runIDsTest(t, dir, args...)
		for _, want := range []string{"XXXXXX: area", `tiki exec 'update where id = "XXXXXX" set area="feature"'`, "tiki ids migrate"} {
			if !strings.Contains(errOut, want) {
				t.Errorf("ids %v stderr lacks %q:\n%s", args, want, errOut)
			}
		}
		if code != exitQueryError || strings.Contains(errOut, "NNNNNN") {
			t.Errorf("ids %v: exit %d (want %d), notes-only tiki must not be listed:\n%s", args, code, exitQueryError, errOut)
		}
	}
	if fileText(t, filepath.Join(dir, "a.md")) != before {
		t.Error("a refused migrate changed a.md")
	}
}

func TestIDsCounterShowsThisProjectsFile(t *testing.T) {
	dir := setupIDsProject(t, idsWorkflow, map[string]string{
		"a.md": "---\nid: XXXXXX\ntitle: a\nstatus: open\narea: feature\n---\n",
	})
	code, out, errOut := runIDsTest(t, dir, "counter")
	if code != exitOK || !strings.Contains(out, "counter file: ") || !strings.Contains(out, "not created yet") {
		t.Fatalf("before any prefixed id: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if code, _, errOut := runIDsTest(t, dir, "rename", "XXXXXX", "FEAT-55"); code != exitOK {
		t.Fatalf("rename: exit %d, %s", code, errOut)
	}
	code, out, _ = runIDsTest(t, dir, "counter")
	wantPrefix := filepath.Join(os.Getenv("XDG_STATE_HOME"), "tiki", "sequences", filepath.Base(dir)+"-")
	if code != exitOK || !strings.Contains(out, "counter file: "+wantPrefix) || !strings.Contains(out, "  FEAT: 55") || strings.Contains(out, "not created") {
		t.Errorf("after the rename: exit %d, output:\n%s\nwant the file under %s… holding FEAT: 55", code, out, wantPrefix)
	}
}

func TestIDsCounterRenamesAnOlderFile(t *testing.T) {
	dir := setupIDsProject(t, idsWorkflow, map[string]string{
		"a.md": "---\nid: XXXXXX\ntitle: a\nstatus: open\narea: feature\n---\n",
	})
	if code, _, errOut := runIDsTest(t, dir, "rename", "XXXXXX", "FEAT-55"); code != exitOK {
		t.Fatalf("rename: exit %d, %s", code, errOut)
	}
	seq := filepath.Join(os.Getenv("XDG_STATE_HOME"), "tiki", "sequences")
	current, err := filepath.Glob(filepath.Join(seq, "*.yaml"))
	if err != nil || len(current) != 1 {
		t.Fatalf("counter files = %v, %v", current, err)
	}
	hash := strings.TrimPrefix(filepath.Base(current[0]), filepath.Base(dir)+"-")
	older := filepath.Join(seq, "commit-"+hash)
	//nolint:gosec // G703: paths inside the test's own temporary state dir
	if err := os.Rename(current[0], older); err != nil {
		t.Fatal(err)
	}
	code, out, _ := runIDsTest(t, dir, "counter")
	if code != exitOK || !strings.Contains(out, "counter file: "+current[0]) || !strings.Contains(out, "  FEAT: 55") {
		t.Errorf("exit %d, output:\n%s\nwant %s holding FEAT: 55", code, out, current[0])
	}
	//nolint:gosec // G703: a path inside the test's own temporary state dir
	if _, err := os.Stat(older); !os.IsNotExist(err) {
		t.Errorf("the older file is still there (stat err %v)", err)
	}
}
