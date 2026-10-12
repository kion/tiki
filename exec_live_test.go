package main

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/boolean-maybe/tiki/config"
)

// An installed tiki would otherwise run for the triggers, possibly an older version.
func stubTikiOnPath(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return ""
	}
	stub, calls := writeTikiStub(t)
	t.Setenv("PATH", filepath.Dir(stub)+string(os.PathListSeparator)+os.Getenv("PATH"))
	return calls
}

func writeTikiStub(t *testing.T) (stub, calls string) {
	t.Helper()
	dir := t.TempDir()
	stub = filepath.Join(dir, "tiki")
	calls = filepath.Join(dir, "calls")
	script := "#!/bin/sh\necho \"$@\" >> \"" + calls + "\"\n"
	//nolint:gosec // G306: the stub must be executable
	if err := os.WriteFile(stub, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return stub, calls
}

func setupExecProject(t *testing.T, workflow string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	config.ResetPathManager()
	t.Cleanup(config.ResetPathManager)
	yaml, ok := config.LookupEmbeddedWorkflow(workflow)
	if !ok {
		t.Fatalf("no embedded workflow %q", workflow)
	}
	if err := os.WriteFile("workflow.yaml", []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestBundledWorkflows_ClearWithTheRunningTiki(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("run() triggers go through sh")
	}
	stub, calls := writeTikiStub(t)
	t.Setenv(tikiBinEnv, stub)
	setupExecProject(t, "kanban")

	for _, stmt := range []string{
		`create title="Main" assignee="me" status="inProgress"`,
		`update where title = "Main" set status="done"`,
	} {
		if got := runExec([]string{stmt}); got != exitOK {
			t.Fatalf("exec %q: exit %d", stmt, got)
		}
	}
	if got := stubCalls(t, calls); len(got) != 1 {
		t.Fatalf("calls to %s = %q, want the trigger to use it rather than PATH", tikiBinEnv, got)
	}
}

func TestExportTikiBin(t *testing.T) {
	t.Setenv(tikiBinEnv, "")
	exportTikiBin()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(tikiBinEnv); got != exe {
		t.Fatalf("%s = %q, want this executable %q", tikiBinEnv, got, exe)
	}

	t.Setenv(tikiBinEnv, "/opt/other/tiki")
	exportTikiBin()
	if got := os.Getenv(tikiBinEnv); got != "/opt/other/tiki" {
		t.Fatalf("%s = %q, want the caller's value kept", tikiBinEnv, got)
	}
}

func stubCalls(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func TestBundledWorkflows_ClearLiveStatusWhenClosed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("run() triggers go through sh")
	}
	tests := []struct {
		workflow string
		steps    []string
	}{
		{workflow: "kanban", steps: []string{
			`create title="Main" assignee="me" status="inProgress"`,
			`update where title = "Main" set status="done"`,
			`update where title = "Main" set tags=tags + ["edited"]`,
		}},
		{workflow: "todo", steps: []string{
			`create title="Main" status="todo"`,
			`update where title = "Main" set status="done"`,
			`update where title = "Main" set priority=1`,
		}},
		{workflow: "bug-tracker", steps: []string{
			`create title="Main" status="inReview"`,
			`update where title = "Main" set status="verified"`,
			`update where title = "Main" set tags=tags + ["edited"]`,
		}},
		{workflow: "bug-tracker", steps: []string{
			`create title="Main" status="open"`,
			`update where title = "Main" set status="wontFix"`,
		}},
	}
	clearCall := regexp.MustCompile(`^live clear [A-Z0-9]{6}$`)
	for _, tt := range tests {
		t.Run(tt.workflow, func(t *testing.T) {
			calls := stubTikiOnPath(t)
			setupExecProject(t, tt.workflow)

			for _, stmt := range tt.steps {
				if got := runExec([]string{stmt}); got != exitOK {
					t.Fatalf("exec %q: exit %d", stmt, got)
				}
			}
			got := stubCalls(t, calls)
			if len(got) != 1 || !clearCall.MatchString(got[0]) {
				t.Fatalf("tiki calls = %q, want one `live clear <ID>` on closing", got)
			}
		})
	}
}
