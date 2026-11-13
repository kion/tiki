package config

import (
	"os"
	"path/filepath"
	"testing"
)

func useTempCacheHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", home)
	ResetPathManager()
	t.Cleanup(ResetPathManager)
	return filepath.Join(home, "tiki", "state.yaml")
}

func TestUIState_MissingFileIsDefaults(t *testing.T) {
	useTempCacheHome(t)
	if got := LoadUIState(); got != (UIState{}) {
		t.Errorf("LoadUIState() = %+v, want zero value", got)
	}
}

func TestUIState_SaveCreatesTheDirAndRoundTrips(t *testing.T) {
	path := useTempCacheHome(t)

	want := UIState{Detail: DetailUIState{BodyMode: BodyModeSource}}
	if err := SaveUIState(want); err != nil {
		t.Fatalf("SaveUIState: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("state file not written at %s: %v", path, err)
	}
	if got := LoadUIState(); got != want {
		t.Errorf("LoadUIState() = %+v, want %+v", got, want)
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("temp files left behind: %v", entries)
	}
}

func TestUIState_CorruptFileIsDefaults(t *testing.T) {
	path := useTempCacheHome(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("detail: [not: a mapping"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := LoadUIState(); got != (UIState{}) {
		t.Errorf("LoadUIState() = %+v, want zero value", got)
	}
}
