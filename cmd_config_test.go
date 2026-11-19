package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/boolean-maybe/tiki/config"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func configReport(t *testing.T) string {
	t.Helper()
	var code int
	out := captureStdout(t, func() { code = runConfig(nil) })
	if code != exitOK {
		t.Fatalf("runConfig exit = %d, want %d", code, exitOK)
	}
	return out
}

// assertRow checks a report row for scope, file and status, ignoring column padding.
func assertRow(t *testing.T, out, scope, path, status string) {
	t.Helper()
	re := regexp.MustCompile(`(?m)^  ` + regexp.QuoteMeta(scope) + ` +` + regexp.QuoteMeta(path) + ` +` + regexp.QuoteMeta(status) + `$`)
	if !re.MatchString(out) {
		t.Errorf("no row %q %q %q in:\n%s", scope, path, status, out)
	}
}

func TestConfigReport_StatusesAndReasons(t *testing.T) {
	tikiDir := setupWorkflowTest(t)
	cwd, _ := os.Getwd()
	writeFile(t, filepath.Join(tikiDir, config.LegacyConfigFileName), "logging:\n  level: info\n")
	writeFile(t, filepath.Join(tikiDir, config.LegacyWorkflowFileName), "fields: []\n")
	writeFile(t, filepath.Join(cwd, config.ConfigFileName), "project:\n  name: Acme\n")
	writeFile(t, filepath.Join(cwd, config.LegacyConfigFileName), "logging:\n  level: debug\n")
	writeFile(t, filepath.Join(cwd, config.LegacyWorkflowFileName), "name: CI\non: push\n")

	out := configReport(t)

	assertRow(t, out, "global", filepath.Join(tikiDir, config.LegacyConfigFileName), "loaded")
	assertRow(t, out, "local", filepath.Join(cwd, config.ConfigFileName), "loaded")
	assertRow(t, out, "local", filepath.Join(cwd, config.LegacyConfigFileName), "ignored: tiki-config.yaml takes precedence")
	assertRow(t, out, "global", filepath.Join(tikiDir, config.LegacyWorkflowFileName), "loaded")
	assertRow(t, out, "local", filepath.Join(cwd, config.LegacyWorkflowFileName), "skipped: no tiki workflow keys")
	if !strings.HasPrefix(out, "config\n") || !strings.Contains(out, "\nworkflow\n") {
		t.Errorf("report should have config and workflow sections:\n%s", out)
	}
}

func TestConfigReport_LocalWorkflowOverridesGlobal(t *testing.T) {
	tikiDir := setupWorkflowTest(t)
	cwd, _ := os.Getwd()
	writeFile(t, filepath.Join(tikiDir, config.WorkflowFileName), "fields: []\n")
	writeFile(t, filepath.Join(cwd, config.WorkflowFileName), "fields: []\n")

	out := configReport(t)

	assertRow(t, out, "global", filepath.Join(tikiDir, config.WorkflowFileName), "overridden by local")
	assertRow(t, out, "local", filepath.Join(cwd, config.WorkflowFileName), "loaded")
}

func TestConfigReport_Missing(t *testing.T) {
	tikiDir := setupWorkflowTest(t)
	cwd, _ := os.Getwd()

	out := configReport(t)

	assertRow(t, out, "global", filepath.Join(tikiDir, config.ConfigFileName), "missing")
	assertRow(t, out, "local", filepath.Join(cwd, config.WorkflowFileName), "missing")
}

func TestConfigReport_FlagsPersonalSettingsInLocalFile(t *testing.T) {
	setupWorkflowTest(t)
	cwd, _ := os.Getwd()
	local := filepath.Join(cwd, config.ConfigFileName)
	writeFile(t, local, "identity:\n  name: Sam\nlogging:\n  level: debug\nappearance:\n  theme: nord\n")

	out := configReport(t)

	want := "  note: identity, logging in " + local + " apply to everyone who uses this project\n"
	if !strings.Contains(out, want) {
		t.Errorf("missing personal-settings note %q in:\n%s", want, out)
	}
}

func TestRunConfig_Usage(t *testing.T) {
	setupWorkflowTest(t)
	var code int
	out := captureStdout(t, func() { code = runConfig([]string{"--help"}) })
	if code != exitOK || !strings.Contains(out, "Usage: tiki config") {
		t.Errorf("--help: exit %d, output %q", code, out)
	}
	if code := runConfig([]string{"extra"}); code != exitUsage {
		t.Errorf("unexpected argument: exit %d, want %d", code, exitUsage)
	}
}
