package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const intrinsicSchemaBlock = `intrinsic fields (every tiki):
  id           id         assigned on create, immutable
  title        string     required on create
  description  string     the Markdown body
  createdBy    string     set by tiki
  createdAt    timestamp  set by tiki
  updatedAt    timestamp  set by tiki
  activeAt     timestamp  computed: the later of updatedAt and the live status time
  filepath     string     computed: absolute path on disk
`

func workflowSchemaOutput(t *testing.T) string {
	t.Helper()
	var code int
	out := captureStdout(t, func() { code = runWorkflow([]string{"schema"}) })
	if code != exitOK {
		t.Fatalf("workflow schema: exit %d, want %d\n%s", code, exitOK, out)
	}
	return out
}

func writeCwdWorkflow(t *testing.T, content string) {
	t.Helper()
	if err := os.WriteFile("workflow.yaml", []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowSchema_CustomWorkflow(t *testing.T) {
	setupWorkflowTest(t)
	writeCwdWorkflow(t, `version: "0.6.1"
fields:
  - name: stage
    type: enum
    caption: "Stage"
    values:
      - {value: open, label: Open, visual: "○", default: true}
      - {value: "on hold", label: "On Hold"}
      - {value: closed, label: Closed}
  - name: tally
    type: text
  - name: effort
    type: integer
    default: 3
  - name: labels
    type: stringList
    default: ["new"]
  - name: blockers
    type: tikiIdList
  - name: review
    type: date
  - name: cadence
    type: recurrence
  - name: owner
    type: user
views:
  - name: ignored
triggers:
  - description: closed tikis cannot be deleted
    ruki: >
      before delete
        where old.stage = "closed"
        deny "closed tikis stay"
  - ruki: >
      after update
        where new.stage = "closed"
        update where id = new.id set effort = 1
  - description: drop closed tikis after a week
    ruki: >
      every 7day
        delete where stage = "closed"
`)

	out := workflowSchemaOutput(t)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	want := "workflow: " + filepath.Join(cwd, "workflow.yaml") + "\n\n" + intrinsicSchemaBlock + `
workflow fields (* = default on create):
  stage     enum          open* "on hold" closed
  tally     string
  effort    int           default 3
  labels    list<string>  default ["new"]
  blockers  list<ref>     tiki ids
  review    date          YYYY-MM-DD
  cadence   recurrence    daily(), weekly("monday"), monthly(1)
  owner     user          a name, as a string; user() is the current user

triggers (deny refuses the change; the rest are side effects):
  before delete  deny    closed tikis cannot be deleted
  after update   update  after update where new.stage = "closed" update where id = new.id set effort = 1
  every 7day     delete  drop closed tikis after a week
`
	if out != want {
		t.Errorf("schema output mismatch\n--- got ---\n%s\n--- want ---\n%s", out, want)
	}
}

func TestWorkflowSchema_BuiltInWorkflow(t *testing.T) {
	tikiDir := setupWorkflowTest(t)

	out := workflowSchemaOutput(t)

	for _, want := range []string{
		"workflow: " + filepath.Join(tikiDir, "workflow.yaml") + "\n",
		intrinsicSchemaBlock,
		"  status      enum          inbox* ready inProgress done\n",
		"  points      enum          10 9 8 7 6 5 4 3 2 1\n",
		"  tags        list<string>  default [\"idea\"]\n",
		"  dependsOn   list<ref>     tiki ids\n",
		"  before update  deny    block completion with open dependencies\n",
		"  after delete   update  remove deleted task from dependency lists\n",
		"  after update   create  spawn next occurrence when recurring task completes\n",
		"  after update   run     clear the agent live status when a tiki is done\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("built-in schema is missing %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"views", "Board", "caption", "In Progress", "📥", "<action>"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("built-in schema should not mention %q:\n%s", unwanted, out)
		}
	}
	if len(out) > 2048 {
		t.Errorf("built-in schema is %d bytes, want at most 2048:\n%s", len(out), out)
	}
}

func TestWorkflowSchema_NoFieldsOrTriggers(t *testing.T) {
	setupWorkflowTest(t)
	writeCwdWorkflow(t, "version: \"0.6.1\"\n")

	out := workflowSchemaOutput(t)

	if !strings.Contains(out, "\nworkflow fields: none\n") || !strings.Contains(out, "\ntriggers: none\n") {
		t.Errorf("empty workflow should report no fields and no triggers:\n%s", out)
	}
}

func TestWorkflowSchema_BrokenTriggerFails(t *testing.T) {
	setupWorkflowTest(t)
	writeCwdWorkflow(t, `version: "0.6.1"
triggers:
  - description: refers to a field the workflow lacks
    ruki: before update where new.nosuch = "x" deny "no"
`)

	var code int
	captureStdout(t, func() { code = runWorkflow([]string{"schema"}) })
	if code != exitInternal {
		t.Errorf("broken trigger: exit %d, want %d", code, exitInternal)
	}
}

func TestWorkflowSchema_Args(t *testing.T) {
	setupWorkflowTest(t)

	tests := []struct {
		name string
		args []string
		want int
	}{
		{"help", []string{"schema", "--help"}, exitOK},
		{"unexpected argument", []string{"schema", "kanban"}, exitUsage},
		{"unknown flag", []string{"schema", "--format"}, exitUsage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var code int
			out := captureStdout(t, func() { code = runWorkflow(tt.args) })
			if code != tt.want {
				t.Errorf("exit %d, want %d", code, tt.want)
			}
			if strings.Contains(out, "intrinsic fields") {
				t.Errorf("should print usage, not a schema:\n%s", out)
			}
		})
	}
}
