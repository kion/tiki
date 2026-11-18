package config

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestEmbeddedWorkflowNames_MatchSourceFiles ensures the registered embedded
// workflow names stay in sync with the YAML sources under config/workflows/.
func TestEmbeddedWorkflowNames_MatchSourceFiles(t *testing.T) {
	matches, err := filepath.Glob("workflows/*.yaml")
	if err != nil {
		t.Fatalf("glob embedded workflow files: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no embedded workflow files found at workflows/*.yaml")
	}

	names := make([]string, 0, len(matches))
	for _, path := range matches {
		base := filepath.Base(path)
		names = append(names, strings.TrimSuffix(base, filepath.Ext(base)))
	}
	sort.Strings(names)

	if got := EmbeddedWorkflowNames(); !reflect.DeepEqual(got, names) {
		t.Fatalf("EmbeddedWorkflowNames() = %v, want %v", got, names)
	}
}

// TestEmbeddedWorkflows_MatchSourceFiles ensures each registered embedded
// workflow matches its source file under config/workflows/.
func TestEmbeddedWorkflows_MatchSourceFiles(t *testing.T) {
	for _, name := range EmbeddedWorkflowNames() {
		t.Run(name, func(t *testing.T) {
			embedded, ok := LookupEmbeddedWorkflow(name)
			if !ok {
				t.Fatalf("embedded workflow %q not found", name)
			}

			sourcePath := filepath.Join("workflows", name+".yaml")
			source, err := os.ReadFile(sourcePath)
			if err != nil {
				t.Fatalf("read source %s: %v", sourcePath, err)
			}

			if embedded != string(source) {
				t.Errorf("embedded %q differs from source %s", name, sourcePath)
			}
		})
	}
}

// TestEmbeddedWorkflows_HaveDescription validates each embedded workflow has a
// parseable description field and source file that parses as a full workflow.
func TestEmbeddedWorkflows_HaveDescription(t *testing.T) {
	for _, name := range EmbeddedWorkflowNames() {
		t.Run(name, func(t *testing.T) {
			content, _ := LookupEmbeddedWorkflow(name)
			desc, err := DescribeWorkflowContent(content)
			if err != nil {
				t.Fatalf("parse embedded %s: %v", name, err)
			}
			if desc == "" {
				t.Errorf("embedded %s has empty description", name)
			}

			sourcePath := filepath.Join("workflows", name+".yaml")
			data, err := os.ReadFile(sourcePath)
			if err != nil {
				t.Fatalf("read %s: %v", sourcePath, err)
			}

			var sourceDesc struct {
				Description string `yaml:"description"`
			}
			if err := yaml.Unmarshal(data, &sourceDesc); err != nil {
				t.Fatalf("unmarshal description from %s: %v", sourcePath, err)
			}
			if sourceDesc.Description == "" {
				t.Errorf("%s: missing or empty top-level description", sourcePath)
			}

			if _, err := readWorkflowFile(sourcePath); err != nil {
				t.Errorf("readWorkflowFile(%s) failed: %v", sourcePath, err)
			}
		})
	}
}

func TestEmbeddedWorkflows_HaveExpectedVersionAndUserFields(t *testing.T) {
	tests := []struct {
		name           string
		wantVersion    string
		wantAssignee   string
		wantReportedBy string
	}{
		{name: "kanban", wantVersion: "0.6.1", wantAssignee: "user"},
		{name: "bug-tracker", wantVersion: "0.6.1", wantAssignee: "user", wantReportedBy: "text"},
		{name: "todo", wantVersion: "0.6.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wf, err := readWorkflowFile(filepath.Join("workflows", tt.name+".yaml"))
			if err != nil {
				t.Fatalf("read workflow: %v", err)
			}
			if wf.Version != tt.wantVersion {
				t.Fatalf("version = %q, want %q", wf.Version, tt.wantVersion)
			}
			types := map[string]string{}
			for _, field := range wf.Fields {
				name, _ := field["name"].(string)
				typ, _ := field["type"].(string)
				types[name] = typ
			}
			if tt.wantAssignee == "" {
				if _, ok := types["assignee"]; ok {
					t.Fatal("assignee field should not exist")
				}
			} else if got := types["assignee"]; got != tt.wantAssignee {
				t.Fatalf("assignee type = %q, want %q", got, tt.wantAssignee)
			}
			if tt.wantReportedBy != "" {
				if got := types["reportedBy"]; got != tt.wantReportedBy {
					t.Fatalf("reportedBy type = %q, want %q", got, tt.wantReportedBy)
				}
			}
		})
	}
}

func uncommentIDExample(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if !strings.HasSuffix(strings.TrimSpace(line), "#id-example") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		body := strings.TrimPrefix(strings.TrimLeft(line, " "), "# ")
		body = strings.TrimSpace(strings.Replace(body, "#id-example", "", 1))
		lines[i] = strings.Repeat(" ", indent) + body
	}
	return strings.Join(lines, "\n")
}

func TestEmbeddedWorkflows_IDExampleIsOffAndWorksWhenUncommented(t *testing.T) {
	tests := []struct {
		name         string
		prefix       string
		typePrefixes map[string]string
	}{
		{"kanban", "TIK", map[string]string{"story": "STORY", "bug": "BUG", "spike": "SPIKE", "project": "PROJ"}},
		{"bug-tracker", "TIK", map[string]string{"bug": "BUG", "regression": "REG", "incident": "INC"}},
		{"todo", "TODO", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, ok := LookupEmbeddedWorkflow(tt.name)
			if !ok {
				t.Fatal("not embedded")
			}
			if !strings.Contains(content, "#id-example") {
				t.Fatal("no #id-example lines")
			}
			dir := t.TempDir()
			shipped := filepath.Join(dir, "shipped.yaml")
			if err := os.WriteFile(shipped, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			if fd, err := readCustomFieldsFromFile(shipped); err != nil || fd.Prefix != "" {
				t.Fatalf("shipped workflow must keep random ids: prefix %q, err %v", fd.Prefix, err)
			}

			on := filepath.Join(dir, "on.yaml")
			if err := os.WriteFile(on, []byte(uncommentIDExample(content)), 0o644); err != nil {
				t.Fatal(err)
			}
			defs, err := LoadWorkflowFieldsFromFile(on)
			if err != nil {
				t.Fatalf("uncommented example does not load: %v", err)
			}
			fd, err := readCustomFieldsFromFile(on)
			if err != nil || fd.Prefix != tt.prefix {
				t.Fatalf("top-level prefix = %q (err %v), want %q", fd.Prefix, err, tt.prefix)
			}
			for _, d := range defs {
				if d.Name != "type" {
					continue
				}
				for _, v := range d.EnumValues {
					if v.Prefix != tt.typePrefixes[v.Value] {
						t.Errorf("type %s prefix = %q, want %q", v.Value, v.Prefix, tt.typePrefixes[v.Value])
					}
				}
			}
		})
	}
}
