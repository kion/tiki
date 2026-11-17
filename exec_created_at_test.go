package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/boolean-maybe/tiki/config"
)

func TestRunExec_CreateWritesCreatedAt(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	config.ResetPathManager()
	t.Cleanup(config.ResetPathManager)

	if got := runExec([]string{`create title="Fresh"`}); got != exitOK {
		t.Fatalf("exec create: exit %d, want %d", got, exitOK)
	}
	files, err := filepath.Glob("*.md")
	if err != nil || len(files) != 1 {
		t.Fatalf("want one .md file, got %v (err %v)", files, err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read %s: %v", files[0], err)
	}
	if !regexp.MustCompile(`(?m)^createdAt: \d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$`).Match(data) {
		t.Errorf("created file has no createdAt line:\n%s", data)
	}
}
