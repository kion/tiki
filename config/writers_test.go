package config

import (
	"os"
	"path/filepath"
	"testing"
)

func chdirTemp(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	return dir
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Errorf("%s should not exist (err=%v)", path, err)
	}
}

func assertContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Errorf("%s = %q, want %q", path, got, want)
	}
}

func TestInstallWorkflowFromContent_EmptyLocalWritesTikiName(t *testing.T) {
	setupResetTest(t)
	cwd := chdirTemp(t)

	if _, err := InstallWorkflowFromContent("fields: []\n", ScopeCurrent); err != nil {
		t.Fatal(err)
	}

	assertContent(t, filepath.Join(cwd, WorkflowFileName), "fields: []\n")
	assertMissing(t, filepath.Join(cwd, LegacyWorkflowFileName))
}

func TestInstallWorkflowFromContent_OverwritesRecognisedLegacy(t *testing.T) {
	setupResetTest(t)
	cwd := chdirTemp(t)
	writeTestFile(t, filepath.Join(cwd, LegacyWorkflowFileName), "fields: []\n")

	if _, err := InstallWorkflowFromContent("views: []\n", ScopeCurrent); err != nil {
		t.Fatal(err)
	}

	assertContent(t, filepath.Join(cwd, LegacyWorkflowFileName), "views: []\n")
	assertMissing(t, filepath.Join(cwd, WorkflowFileName))
}

func TestInstallWorkflowFromContent_ForeignLegacyWritesBeside(t *testing.T) {
	setupResetTest(t)
	cwd := chdirTemp(t)
	foreign := "name: CI\non: push\n"
	writeTestFile(t, filepath.Join(cwd, LegacyWorkflowFileName), foreign)

	if _, err := InstallWorkflowFromContent("fields: []\n", ScopeCurrent); err != nil {
		t.Fatal(err)
	}

	assertContent(t, filepath.Join(cwd, LegacyWorkflowFileName), foreign)
	assertContent(t, filepath.Join(cwd, WorkflowFileName), "fields: []\n")
}

func TestInstallWorkflowFromContent_RefusesLocalSymlink(t *testing.T) {
	setupResetTest(t)
	cwd := chdirTemp(t)
	victim := filepath.Join(t.TempDir(), "bashrc")
	writeTestFile(t, victim, "keep me\n")
	if err := os.Symlink(victim, filepath.Join(cwd, WorkflowFileName)); err != nil {
		t.Skip("symlinks unavailable:", err)
	}

	if _, err := InstallWorkflowFromContent("fields: []\n", ScopeCurrent); err == nil {
		t.Fatal("expected an error for a symlinked project workflow")
	}

	assertContent(t, victim, "keep me\n")
}

func TestInstallWorkflowFromContent_FollowsGlobalSymlink(t *testing.T) {
	tikiDir := setupResetTest(t)
	dotfile := filepath.Join(t.TempDir(), "workflow.yaml")
	writeTestFile(t, dotfile, "fields: []\n")
	if err := os.Symlink(dotfile, filepath.Join(tikiDir, LegacyWorkflowFileName)); err != nil {
		t.Skip("symlinks unavailable:", err)
	}

	if _, err := InstallWorkflowFromContent("views: []\n", ScopeGlobal); err != nil {
		t.Fatal(err)
	}

	assertContent(t, dotfile, "views: []\n")
}

func TestResetConfig_CurrentKeepsForeignFiles(t *testing.T) {
	setupResetTest(t)
	cwd := chdirTemp(t)
	files := map[string]string{
		LegacyConfigFileName:   "server:\n  port: 80\n",
		LegacyWorkflowFileName: "name: CI\non: push\n",
	}
	for name, content := range files {
		writeTestFile(t, filepath.Join(cwd, name), content)
	}

	affected, err := ResetConfig(ScopeCurrent, TargetAll)
	if err != nil {
		t.Fatal(err)
	}

	if len(affected) != 0 {
		t.Errorf("affected = %v, want none", affected)
	}
	for name, content := range files {
		assertContent(t, filepath.Join(cwd, name), content)
	}
}

func TestResetConfig_CurrentKeepsInvalidLegacy(t *testing.T) {
	setupResetTest(t)
	cwd := chdirTemp(t)
	templated := "{% if prod %}\nfields: []\n{% endif %}\n"
	writeTestFile(t, filepath.Join(cwd, LegacyWorkflowFileName), templated)

	if _, err := ResetConfig(ScopeCurrent, TargetWorkflow); err != nil {
		t.Fatal(err)
	}

	assertContent(t, filepath.Join(cwd, LegacyWorkflowFileName), templated)
}

func TestResetConfig_CurrentDeletesBothRecognisedNames(t *testing.T) {
	setupResetTest(t)
	cwd := chdirTemp(t)
	writeTestFile(t, filepath.Join(cwd, WorkflowFileName), "fields: []\n")
	writeTestFile(t, filepath.Join(cwd, LegacyWorkflowFileName), "fields: []\n")

	affected, err := ResetConfig(ScopeCurrent, TargetWorkflow)
	if err != nil {
		t.Fatal(err)
	}

	if len(affected) != 2 {
		t.Errorf("affected = %v, want both names", affected)
	}
	assertMissing(t, filepath.Join(cwd, WorkflowFileName))
	assertMissing(t, filepath.Join(cwd, LegacyWorkflowFileName))
}

func TestResetConfig_CurrentRemovesSymlinkNotTarget(t *testing.T) {
	setupResetTest(t)
	cwd := chdirTemp(t)
	victim := filepath.Join(t.TempDir(), "victim.yaml")
	writeTestFile(t, victim, "fields: []\n")
	link := filepath.Join(cwd, WorkflowFileName)
	if err := os.Symlink(victim, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}

	if _, err := ResetConfig(ScopeCurrent, TargetWorkflow); err != nil {
		t.Fatal(err)
	}

	assertMissing(t, link)
	assertContent(t, victim, "fields: []\n")
}

func TestInstallDefaultWorkflow_NoopWhenLegacyExists(t *testing.T) {
	tikiDir := setupResetTest(t)
	writeTestFile(t, filepath.Join(tikiDir, LegacyWorkflowFileName), "fields: []\n")

	if err := InstallDefaultWorkflow(); err != nil {
		t.Fatal(err)
	}

	assertMissing(t, filepath.Join(tikiDir, WorkflowFileName))
}

func TestInstallDefaultWorkflow_ThenLoad(t *testing.T) {
	tikiDir := setupResetTest(t)
	chdirTemp(t)
	t.Cleanup(ClearWorkflowFields)

	if err := InstallDefaultWorkflow(); err != nil {
		t.Fatal(err)
	}
	if err := LoadWorkflowFields(); err != nil {
		t.Fatalf("LoadWorkflowFields after seeding: %v", err)
	}

	assertContent(t, filepath.Join(tikiDir, WorkflowFileName), GetDefaultWorkflowYAML())
}
