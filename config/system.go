package config

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/boolean-maybe/tiki/document"
)

//go:embed workflows/kanban.yaml
var embeddedKanbanYAML string

//go:embed workflows/todo.yaml
var embeddedTodoYAML string

//go:embed workflows/bug-tracker.yaml
var embeddedBugTrackerYAML string

const DefaultEmbeddedWorkflowName = "kanban"

var embeddedWorkflows = map[string]string{
	"kanban":      embeddedKanbanYAML,
	"todo":        embeddedTodoYAML,
	"bug-tracker": embeddedBugTrackerYAML,
}

// LookupEmbeddedWorkflow returns the YAML content for a bundled workflow name.
func LookupEmbeddedWorkflow(name string) (string, bool) {
	content, ok := embeddedWorkflows[name]
	return content, ok
}

// EmbeddedWorkflowNames returns the sorted list of bundled workflow names.
func EmbeddedWorkflowNames() []string {
	names := make([]string, 0, len(embeddedWorkflows))
	for k := range embeddedWorkflows {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// GenerateRandomIDForTest is an optional injection point used only by tests
// that need deterministic id sequences (e.g. to exercise collision retries).
// When non-nil, GenerateRandomID delegates here; otherwise it uses
// document.NewID. Production code paths must not set this.
var GenerateRandomIDForTest func() string

// GenerateRandomID generates a bare uppercase document ID (no TIKI- prefix).
// Callers must not prepend "TIKI-" any longer; the identifier is the raw ID.
// Delegates to document.NewID so there is a single authoritative generator.
func GenerateRandomID() string {
	if GenerateRandomIDForTest != nil {
		return GenerateRandomIDForTest()
	}
	return document.NewID()
}

// GetDefaultWorkflowYAML returns the embedded default workflow.yaml content
func GetDefaultWorkflowYAML() string {
	return embeddedKanbanYAML
}

// InstallDefaultWorkflow seeds the user config directory with the default workflow
// when it has no workflow file under either name.
// This runs on every launch to handle first-run and upgrade cases.
func InstallDefaultWorkflow() error {
	dir := GetConfigDir()
	for _, name := range []string{WorkflowFileName, LegacyWorkflowFileName} {
		if exists(filepath.Join(dir, name)) {
			return nil
		}
	}
	path := filepath.Join(dir, WorkflowFileName)

	//nolint:gosec // G301: 0755 is appropriate for config directory
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config directory %s: %w", dir, err)
	}

	//nolint:gosec // G306: 0644 is appropriate for config file
	if err := os.WriteFile(path, []byte(embeddedKanbanYAML), 0644); err != nil {
		return fmt.Errorf("write default workflow %s: %w", path, err)
	}

	return nil
}
