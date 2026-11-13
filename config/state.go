package config

import (
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	BodyModeRendered = "rendered"
	BodyModeSource   = "source"
)

// UIState is what tiki remembers between runs on its own:
// interface choices the user made by using the app, not by writing config.
// It lives in the per-user cache dir, so it follows the user across projects
// and config.yaml is never rewritten.
type UIState struct {
	Detail DetailUIState `yaml:"detail"`
}

// DetailUIState is the detail view's remembered state.
type DetailUIState struct {
	BodyMode string `yaml:"bodyMode"`
}

func uiStatePath() string {
	return filepath.Join(GetCacheDir(), "state.yaml")
}

// LoadUIState reads the saved state.
// A missing or unreadable file gives the defaults:
// losing remembered choices must never stop the app.
func LoadUIState() UIState {
	data, err := os.ReadFile(uiStatePath())
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("ui state: unreadable, using defaults", "path", uiStatePath(), "error", err)
		}
		return UIState{}
	}
	var st UIState
	if err := yaml.Unmarshal(data, &st); err != nil {
		slog.Warn("ui state: corrupt, using defaults", "path", uiStatePath(), "error", err)
		return UIState{}
	}
	return st
}

// SaveUIState replaces the state file by rename, so a crash never leaves it half-written.
func SaveUIState(st UIState) error {
	data, err := yaml.Marshal(st)
	if err != nil {
		return err
	}
	path := uiStatePath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".state.*.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
