package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

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

func TestRunExec_SelectDoesNotStamp(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	t.Chdir(dir)
	config.ResetPathManager()
	t.Cleanup(config.ResetPathManager)
	path := filepath.Join(dir, "a.md")
	content := "---\nid: AAAAAA\ntitle: a\n---\nbody\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Date(2024, 4, 4, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	if got := runExec([]string{`select id`}); got != exitOK {
		t.Fatalf("exec select: exit %d, want %d", got, exitOK)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content || !info.ModTime().Equal(old) {
		t.Errorf("select changed the file (mtime %v):\n%s", info.ModTime(), data)
	}
}
