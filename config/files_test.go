package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFolderFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func candidateFor(t *testing.T, r FolderResolution, name string) Candidate {
	t.Helper()
	for _, c := range r.Candidates {
		if filepath.Base(c.Path) == name {
			return c
		}
	}
	t.Fatalf("no candidate %s in %+v", name, r.Candidates)
	return Candidate{}
}

const tikiWorkflowYAML = "fields:\n  - name: status\n    type: enum\n"

func TestResolveInFolder_TikiNameOnly(t *testing.T) {
	dir := t.TempDir()
	want := writeFolderFile(t, dir, ConfigFileName, "logging:\n  level: debug\n")

	r := ResolveInFolder(KindConfig, dir, false)

	if r.Chosen != want {
		t.Fatalf("Chosen = %q, want %q", r.Chosen, want)
	}
	if c := candidateFor(t, r, ConfigFileName); c.Status != StatusLoaded {
		t.Errorf("status = %q, want loaded", c.Status)
	}
}

func TestResolveInFolder_BothNamesTikiWins(t *testing.T) {
	dir := t.TempDir()
	want := writeFolderFile(t, dir, WorkflowFileName, tikiWorkflowYAML)
	writeFolderFile(t, dir, LegacyWorkflowFileName, tikiWorkflowYAML)

	r := ResolveInFolder(KindWorkflow, dir, false)

	if r.Chosen != want {
		t.Fatalf("Chosen = %q, want %q", r.Chosen, want)
	}
	legacy := candidateFor(t, r, LegacyWorkflowFileName)
	if legacy.Status != StatusIgnored || legacy.Reason != "tiki-workflow.yaml takes precedence" {
		t.Errorf("legacy = %+v, want ignored because tiki-workflow.yaml takes precedence", legacy)
	}
}

func TestResolveInFolder_LegacyRecognisedLocally(t *testing.T) {
	dir := t.TempDir()
	want := writeFolderFile(t, dir, LegacyConfigFileName, "Appearance:\n  theme: nord\nheader:\n  visible: false\n")

	r := ResolveInFolder(KindConfig, dir, false)

	if r.Chosen != want {
		t.Fatalf("Chosen = %q, want %q (keys compare case-insensitively)", r.Chosen, want)
	}
}

func TestResolveInFolder_ForeignLegacyConfigSkipped(t *testing.T) {
	dir := t.TempDir()
	writeFolderFile(t, dir, LegacyConfigFileName, "server:\n  port: 80\nlogging:\n  level: info\ndatabase: x\n")

	r := ResolveInFolder(KindConfig, dir, false)

	if r.Chosen != "" {
		t.Fatalf("Chosen = %q, want none", r.Chosen)
	}
	c := candidateFor(t, r, LegacyConfigFileName)
	if c.Status != StatusSkipped || c.Reason != "not a tiki file (unknown keys: database, server)" {
		t.Errorf("candidate = %+v", c)
	}
}

func TestResolveInFolder_ForeignLegacyWorkflowSkipped(t *testing.T) {
	dir := t.TempDir()
	writeFolderFile(t, dir, LegacyWorkflowFileName, "name: CI\non: push\njobs:\n  build: {}\n")

	r := ResolveInFolder(KindWorkflow, dir, false)

	if r.Chosen != "" {
		t.Fatalf("Chosen = %q, want none", r.Chosen)
	}
	if c := candidateFor(t, r, LegacyWorkflowFileName); c.Status != StatusSkipped || c.Reason != "no tiki workflow keys" {
		t.Errorf("candidate = %+v", c)
	}
}

func TestResolveInFolder_LegacyStatusesWorkflowRecognised(t *testing.T) {
	dir := t.TempDir()
	want := writeFolderFile(t, dir, LegacyWorkflowFileName, "statuses:\n  - key: todo\n")

	if r := ResolveInFolder(KindWorkflow, dir, false); r.Chosen != want {
		t.Fatalf("Chosen = %q, want %q so the migration error is reported", r.Chosen, want)
	}
}

func TestResolveInFolder_InvalidYAMLLegacyCountsAsTiki(t *testing.T) {
	dir := t.TempDir()
	want := writeFolderFile(t, dir, LegacyWorkflowFileName, "fields: [\n")

	if r := ResolveInFolder(KindWorkflow, dir, false); r.Chosen != want {
		t.Fatalf("Chosen = %q, want %q so the parse error is reported", r.Chosen, want)
	}
}

func TestResolveInFolder_EmptyAndNonMapLegacyNotTiki(t *testing.T) {
	for name, content := range map[string]string{"empty": "", "list": "- a\n- b\n", "scalar": "hello\n"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFolderFile(t, dir, LegacyWorkflowFileName, content)
			r := ResolveInFolder(KindWorkflow, dir, false)
			if r.Chosen != "" {
				t.Fatalf("Chosen = %q, want none", r.Chosen)
			}
			if c := candidateFor(t, r, LegacyWorkflowFileName); c.Status != StatusSkipped {
				t.Errorf("status = %q, want skipped", c.Status)
			}
		})
	}
}

func TestResolveInFolder_GlobalLegacyAcceptedAsIs(t *testing.T) {
	dir := t.TempDir()
	want := writeFolderFile(t, dir, LegacyConfigFileName, "server: 1\n")

	if r := ResolveInFolder(KindConfig, dir, true); r.Chosen != want {
		t.Fatalf("Chosen = %q, want %q: the global folder skips recognition", r.Chosen, want)
	}
}

func TestResolveInFolder_NotARegularFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ConfigFileName), 0o750); err != nil {
		t.Fatal(err)
	}
	writeFolderFile(t, dir, LegacyConfigFileName, "logging:\n  level: debug\n")

	r := ResolveInFolder(KindConfig, dir, false)

	if r.Chosen != "" {
		t.Fatalf("Chosen = %q, want none", r.Chosen)
	}
	if c := candidateFor(t, r, ConfigFileName); c.Status != StatusSkipped || c.Reason != "not a regular file" {
		t.Errorf("tiki candidate = %+v", c)
	}
	if c := candidateFor(t, r, LegacyConfigFileName); c.Status != StatusIgnored {
		t.Errorf("legacy status = %q, want ignored (never both)", c.Status)
	}
}

func TestResolveInFolder_Oversized(t *testing.T) {
	dir := t.TempDir()
	writeFolderFile(t, dir, WorkflowFileName, "fields: []\n#"+strings.Repeat("x", maxTikiFileSize))

	r := ResolveInFolder(KindWorkflow, dir, false)

	if r.Chosen != "" {
		t.Fatalf("Chosen = %q, want none", r.Chosen)
	}
	if c := candidateFor(t, r, WorkflowFileName); c.Status != StatusSkipped || c.Reason != "larger than 1 MiB" {
		t.Errorf("candidate = %+v", c)
	}
}

func TestResolveInFolder_NeitherFileIsMissing(t *testing.T) {
	dir := t.TempDir()

	r := ResolveInFolder(KindConfig, dir, true)

	if r.Chosen != "" || len(r.Candidates) != 1 {
		t.Fatalf("resolution = %+v, want one missing candidate", r)
	}
	if c := r.Candidates[0]; c.Status != StatusMissing || filepath.Base(c.Path) != ConfigFileName {
		t.Errorf("candidate = %+v", c)
	}
}

func TestIsDeletableTikiFile(t *testing.T) {
	dir := t.TempDir()
	tiki := writeFolderFile(t, dir, WorkflowFileName, "fields: [\n")
	broken := writeFolderFile(t, dir, LegacyWorkflowFileName, "fields: [\n")
	other := t.TempDir()
	foreign := writeFolderFile(t, other, LegacyConfigFileName, "server: 1\n")
	ours := writeFolderFile(t, t.TempDir(), LegacyConfigFileName, "logging:\n  level: info\n")

	cases := []struct {
		name   string
		kind   FileKind
		path   string
		global bool
		want   bool
	}{
		{"tiki name always", KindWorkflow, tiki, false, true},
		{"invalid YAML legacy local", KindWorkflow, broken, false, false},
		{"invalid YAML legacy global", KindWorkflow, broken, true, true},
		{"foreign legacy local", KindConfig, foreign, false, false},
		{"recognised legacy local", KindConfig, ours, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsDeletableTikiFile(tc.kind, tc.path, tc.global); got != tc.want {
				t.Errorf("IsDeletableTikiFile = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestKnownConfigKeysIncludeProject(t *testing.T) {
	keys := knownConfigKeys()
	for _, k := range []string{"appearance", "header", "logging", "identity", "liveStatus", "project"} {
		if !keys[strings.ToLower(k)] {
			t.Errorf("knownConfigKeys missing %q", k)
		}
	}
}

func setupResolveFilesTest(t *testing.T) (userDir, projectDir string) {
	t.Helper()
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	userDir = filepath.Join(xdg, "tiki")
	if err := os.MkdirAll(userDir, 0o750); err != nil {
		t.Fatal(err)
	}
	projectDir = t.TempDir()
	t.Chdir(projectDir)
	ResetPathManager()
	t.Cleanup(ResetPathManager)
	return userDir, projectDir
}

func TestResolveFiles_GlobalAndLocal(t *testing.T) {
	userDir, projectDir := setupResolveFilesTest(t)
	global := writeFolderFile(t, userDir, LegacyWorkflowFileName, tikiWorkflowYAML)
	local := writeFolderFile(t, projectDir, WorkflowFileName, tikiWorkflowYAML)

	r := ResolveFiles(KindWorkflow)

	if r.Global.Chosen != global {
		t.Errorf("Global.Chosen = %q, want %q", r.Global.Chosen, global)
	}
	if r.Local == nil {
		t.Fatal("Local = nil, want the project folder")
	}
	if !sameFile(r.Local.Chosen, local) {
		t.Errorf("Local.Chosen = %q, want %q", r.Local.Chosen, local)
	}
}

func TestResolveFiles_SymlinkedCwdToUserFolderIsGlobal(t *testing.T) {
	userDir, _ := setupResolveFilesTest(t)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(userDir, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	t.Chdir(link)
	ResetPathManager()

	if r := ResolveFiles(KindConfig); r.Local != nil {
		t.Fatalf("Local = %+v, want nil: the cwd is the user folder", r.Local)
	}
}

func sameFile(a, b string) bool {
	ai, errA := os.Stat(a)
	bi, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(ai, bi)
}

func TestResolutionAll_GlobalThenLocal(t *testing.T) {
	local := FolderResolution{Candidates: []Candidate{{Path: "l"}}}
	r := Resolution{Global: FolderResolution{Candidates: []Candidate{{Path: "g1"}, {Path: "g2"}}}, Local: &local}

	var got []string
	for _, c := range r.All() {
		got = append(got, c.Path)
	}

	if strings.Join(got, ",") != "g1,g2,l" {
		t.Errorf("All() = %v, want g1,g2,l", got)
	}
	if n := len((Resolution{}).All()); n != 0 {
		t.Errorf("empty resolution: %d candidates", n)
	}
}
