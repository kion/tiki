package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/boolean-maybe/tiki/config"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		data, _ := io.ReadAll(r)
		done <- string(data)
	}()
	fn()
	os.Stdout = orig
	_ = w.Close()
	return <-done
}

func execOutput(t *testing.T, args ...string) string {
	t.Helper()
	var code int
	out := captureStdout(t, func() { code = runExec(args) })
	if code != exitOK {
		t.Fatalf("exec %q: exit %d, want %d", args, code, exitOK)
	}
	return out
}

// One `select` over a project used to return every tiki's body,
// which is most of the output and rarely what the caller wants.
func TestRunExec_BodyIsReturnedOnlyWhenAskedFor(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	config.ResetPathManager()
	t.Cleanup(config.ResetPathManager)

	execOutput(t, `create title="Main" description="the body text"`)

	tests := []struct {
		name     string
		args     []string
		wantBody bool
	}{
		{"bare select", []string{`select`}, false},
		{"bare select with where", []string{`select where title = "Main"`}, false},
		{"bare select as json", []string{"--format", "json", `select`}, false},
		{"select star", []string{`select *`}, true},
		{"select star as json", []string{"--format", "json", `select *`}, true},
		{"named description", []string{`select description where title = "Main"`}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := execOutput(t, tt.args...)
			if !strings.Contains(out, "Main") && tt.name != "named description" {
				t.Fatalf("output is missing the tiki:\n%s", out)
			}
			if got := strings.Contains(out, "the body text"); got != tt.wantBody {
				t.Errorf("body present = %v, want %v:\n%s", got, tt.wantBody, out)
			}
		})
	}
}
