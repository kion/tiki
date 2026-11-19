package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Scope identifies which config tier to operate on.
type Scope string

// ResetTarget identifies which config file to reset.
type ResetTarget string

const (
	ScopeGlobal  Scope = "global"
	ScopeCurrent Scope = "current"

	TargetAll      ResetTarget = ""
	TargetConfig   ResetTarget = "config"
	TargetWorkflow ResetTarget = "workflow"
)

// resetEntry pairs a file kind with the default content to restore for global scope.
// If defaultContent is empty, the files are always deleted (no embedded default exists).
type resetEntry struct {
	kind           FileKind
	defaultContent string
}

var resetEntries = []resetEntry{
	// TODO: embed a default config.yaml once one exists; until then, global reset deletes the file
	{kind: KindConfig, defaultContent: ""},
	{kind: KindWorkflow, defaultContent: embeddedKanbanYAML},
}

// ResetConfig resets configuration files for the given scope and target.
// Returns the list of file paths that were actually modified or deleted.
func ResetConfig(scope Scope, target ResetTarget) ([]string, error) {
	dir, err := resolveDir(scope)
	if err != nil {
		return nil, err
	}

	entries, err := filterEntries(target)
	if err != nil {
		return nil, err
	}

	global := scope == ScopeGlobal
	var affected []string
	for _, e := range entries {
		paths, err := resetKind(dir, global, e)
		affected = append(affected, paths...)
		if err != nil {
			return affected, err
		}
	}

	return affected, nil
}

// resetKind restores the global workflow to the default, and otherwise deletes
// every tiki file of the kind in dir under both names, so an old one can't become active again.
// Files that aren't tiki's are never deleted.
func resetKind(dir string, global bool, e resetEntry) ([]string, error) {
	if global && e.defaultContent != "" {
		path, err := WorkflowWriteTarget(dir, true)
		if err != nil {
			return nil, err
		}
		changed, err := writeFileIfChanged(path, e.defaultContent)
		if err != nil {
			return nil, fmt.Errorf("reset %s: %w", path, err)
		}
		if changed {
			return []string{path}, nil
		}
		return nil, nil
	}

	preferred, legacy := e.kind.Names()
	var affected []string
	for _, name := range []string{preferred, legacy} {
		path := filepath.Join(dir, name)
		if _, err := os.Lstat(path); err != nil || !IsDeletableTikiFile(e.kind, path, global) {
			continue
		}
		deleted, err := deleteIfExists(path)
		if err != nil {
			return affected, fmt.Errorf("reset %s: %w", path, err)
		}
		if deleted {
			affected = append(affected, path)
		}
	}
	return affected, nil
}

// ResolveDir returns the directory path for the given scope.
func ResolveDir(scope Scope) (string, error) {
	return resolveDir(scope)
}

// resolveDir returns the directory path for the given scope.
func resolveDir(scope Scope) (string, error) {
	switch scope {
	case ScopeGlobal:
		return GetConfigDir(), nil
	case ScopeCurrent:
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("get working directory: %w", err)
		}
		return cwd, nil
	default:
		return "", fmt.Errorf("unknown scope: %s", scope)
	}
}

// ValidResetTarget reports whether target is a recognized reset target.
func ValidResetTarget(target ResetTarget) bool {
	switch target {
	case TargetAll, TargetConfig, TargetWorkflow:
		return true
	default:
		return false
	}
}

// filterEntries returns the reset entries matching the target.
func filterEntries(target ResetTarget) ([]resetEntry, error) {
	switch target {
	case TargetAll:
		return resetEntries, nil
	case TargetConfig:
		return resetEntries[:1], nil
	case TargetWorkflow:
		return resetEntries[1:], nil
	default:
		return nil, fmt.Errorf("unknown target: %q (use config or workflow)", target)
	}
}

// writeFileIfChanged writes content to path, skipping if the file already has identical content.
// Returns true if the file was actually changed.
func writeFileIfChanged(path string, content string) (bool, error) {
	existing, err := os.ReadFile(path)
	if err == nil && string(existing) == content {
		return false, nil
	}
	return writeFile(path, content)
}

// writeFile writes content to path unconditionally, creating parent dirs if needed.
func writeFile(path string, content string) (bool, error) {
	dir := filepath.Dir(path)
	//nolint:gosec // G301: 0755 is appropriate for config directory
	if err := os.MkdirAll(dir, 0755); err != nil {
		return false, fmt.Errorf("create directory %s: %w", dir, err)
	}
	//nolint:gosec // G306: 0644 is appropriate for config file
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return false, err
	}
	return true, nil
}

// deleteIfExists removes a file if it exists. Returns true if the file was deleted.
func deleteIfExists(path string) (bool, error) {
	err := os.Remove(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}
