package tikistore

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileBirthTime_ReportsCreationNotModification(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.md")
	before := time.Now()
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	after := time.Now()
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	got, ok := fileBirthTime(path, info)
	if !ok {
		t.Skip("filesystem does not record birth time")
	}
	if got.Before(before.Add(-time.Second)) || got.After(after.Add(time.Second)) {
		t.Errorf("birth time = %v, want between %v and %v", got, before, after)
	}
	if got.Equal(old) {
		t.Errorf("birth time = %v, the modification time", got)
	}
}
