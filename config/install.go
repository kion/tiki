package config

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	httpTimeout     = 15 * time.Second
	maxResponseSize = 1 << 20 // 1 MiB
)

// InstallResult describes the outcome for a single installed file.
type InstallResult struct {
	Path    string
	Changed bool
}

// InstallWorkflow fetches workflow content from the given source and writes it
// to the workflow file of the directory for the given scope.
// Callers are responsible for validating content before calling this function
// (use ValidateWorkflowContent + validateWorkflowTriggers for non-embedded sources).
func InstallWorkflow(src WorkflowSource, scope Scope) ([]InstallResult, error) {
	content, err := FetchWorkflowContent(src)
	if err != nil {
		return nil, err
	}
	return InstallWorkflowFromContent(content, scope)
}

// InstallWorkflowFromContent writes pre-fetched (and already validated) workflow
// content to the workflow file of the directory for the given scope.
func InstallWorkflowFromContent(content string, scope Scope) ([]InstallResult, error) {
	dir, err := resolveDir(scope)
	if err != nil {
		return nil, err
	}

	path, err := WorkflowWriteTarget(dir, isUserConfigScope(scope, dir))
	if err != nil {
		return nil, err
	}
	changed, err := writeFileIfChanged(path, content)
	if err != nil {
		return nil, fmt.Errorf("write %s: %w", path, err)
	}

	return []InstallResult{{Path: path, Changed: changed}}, nil
}

// WorkflowWriteTarget returns the workflow file to write in dir:
// the one the folder already uses, so an upgrade never leaves both names, or else the tiki- name.
// In a project folder it refuses anything but a regular file,
// so a cloned repo can't redirect the write through a symlink.
// The user's folder follows symlinks, because dotfile managers link those files.
func WorkflowWriteTarget(dir string, global bool) (string, error) {
	path := ResolveInFolder(KindWorkflow, dir, global).Chosen
	if path == "" {
		path = filepath.Join(dir, WorkflowFileName)
	}
	if err := checkWritable(path, global); err != nil {
		return "", err
	}
	return path, nil
}

func checkWritable(path string, global bool) error {
	stat := os.Lstat
	if global {
		stat = os.Stat
	}
	info, err := stat(path)
	if err != nil {
		return nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to write through symlink %s", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to write %s: not a regular file", path)
	}
	return nil
}

// DescribeWorkflow fetches the workflow content from the given source and
// returns the value of its top-level description field.
func DescribeWorkflow(src WorkflowSource) (string, error) {
	content, err := FetchWorkflowContent(src)
	if err != nil {
		return "", err
	}
	return DescribeWorkflowContent(content)
}

var httpClient = &http.Client{Timeout: httpTimeout}

// isUserConfigScope reports whether a write goes to the user config folder,
// including --current run from inside it, so readers and writers agree on what's global.
func isUserConfigScope(scope Scope, dir string) bool {
	return scope == ScopeGlobal || sameFolder(dir, GetConfigDir())
}
