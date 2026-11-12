package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/boolean-maybe/ruki"
	"github.com/boolean-maybe/tiki/config"
	"github.com/boolean-maybe/tiki/internal/bootstrap"
	"github.com/boolean-maybe/tiki/internal/ruki/runtime"
	"github.com/boolean-maybe/tiki/workflow"
)

var intrinsicFieldNotes = map[string]string{
	"id":          "assigned on create, immutable",
	"title":       "required on create",
	"description": "the Markdown body",
	"createdBy":   "set by tiki",
	"createdAt":   "set by tiki",
	"updatedAt":   "set by tiki",
	"activeAt":    "computed: the later of updatedAt and the live status time",
	"filepath":    "computed: absolute path on disk",
}

// runWorkflowSchema implements `tiki workflow schema`:
// the fields and triggers of the workflow that applies to the cwd, for AI agents.
// It leaves out views, actions, captions and visuals, which an agent has no use for.
func runWorkflowSchema(args []string) int {
	positional, err := parsePositionalOnly(args)
	if errors.Is(err, errHelpRequested) {
		printWorkflowSchemaUsage()
		return exitOK
	}
	if err == nil && positional != "" {
		err = fmt.Errorf("unexpected argument: %q", positional)
	}
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error:", err)
		printWorkflowSchemaUsage()
		return exitUsage
	}

	cfg, err := bootstrap.LoadConfig()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: load config: %v\n", err)
		return exitInternal
	}
	bootstrap.InitCLILogging(cfg)

	// same resolution as `tiki exec`, so the schema matches what statements run against
	if err := config.InstallDefaultWorkflow(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: install default workflow: %v\n", err)
	}
	if err := config.LoadWorkflowFields(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error:", err)
		return exitInternal
	}
	triggers, err := config.LoadTriggerDefs()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error:", err)
		return exitInternal
	}

	if err := writeWorkflowSchema(os.Stdout, config.FindWorkflowFile(), triggers); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error:", err)
		return exitInternal
	}
	return exitOK
}

func writeWorkflowSchema(w io.Writer, path string, triggers []config.TriggerDef) error {
	triggerRows, err := triggerSchemaRows(triggers)
	if err != nil {
		return err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "workflow: %s\n", path)

	b.WriteString("\nintrinsic fields (every tiki):\n")
	var intrinsic [][]string
	for _, fd := range workflow.SystemFields() {
		intrinsic = append(intrinsic, []string{fd.Name, schemaTypeName(fd.Type), intrinsicFieldNotes[fd.Name]})
	}
	writeSchemaRows(&b, intrinsic)

	fields := workflow.WorkflowFields()
	if len(fields) == 0 {
		b.WriteString("\nworkflow fields: none\n")
	} else {
		b.WriteString("\nworkflow fields (* = default on create):\n")
		rows := make([][]string, len(fields))
		for i, fd := range fields {
			rows[i] = []string{fd.Name, schemaTypeName(fd.Type), fieldSchemaNote(fd)}
		}
		writeSchemaRows(&b, rows)
	}

	if len(triggerRows) == 0 {
		b.WriteString("\ntriggers: none\n")
	} else {
		b.WriteString("\ntriggers (deny refuses the change; the rest are side effects):\n")
		writeSchemaRows(&b, triggerRows)
	}

	_, err = io.WriteString(w, b.String())
	return err
}

func writeSchemaRows(b *strings.Builder, rows [][]string) {
	widths := make([]int, 2)
	for _, row := range rows {
		for c := range widths {
			widths[c] = max(widths[c], len(row[c]))
		}
	}
	for _, row := range rows {
		line := fmt.Sprintf("  %-*s  %-*s  %s", widths[0], row[0], widths[1], row[1], row[2])
		b.WriteString(strings.TrimRight(line, " "))
		b.WriteByte('\n')
	}
}

func schemaTypeName(vt workflow.ValueType) string {
	switch vt {
	case workflow.TypeString:
		return "string"
	case workflow.TypeInt:
		return "int"
	case workflow.TypeDate:
		return "date"
	case workflow.TypeTimestamp:
		return "timestamp"
	case workflow.TypeDuration:
		return "duration"
	case workflow.TypeBool:
		return "bool"
	case workflow.TypeID:
		return "id"
	case workflow.TypeRef:
		return "ref"
	case workflow.TypeRecurrence:
		return "recurrence"
	case workflow.TypeListString:
		return "list<string>"
	case workflow.TypeListRef:
		return "list<ref>"
	case workflow.TypeEnum:
		return "enum"
	case workflow.TypeUser:
		return "user"
	default:
		return "unknown"
	}
}

// fieldSchemaNote is what an agent needs to write a value for the field:
// the allowed values of an enum, the literal syntax where the type name doesn't give it away,
// and the creation default.
func fieldSchemaNote(fd workflow.FieldDef) string {
	if fd.Type == workflow.TypeEnum {
		values := make([]string, len(fd.EnumValues))
		for i, v := range fd.EnumValues {
			values[i] = v.Value
			if strings.ContainsAny(v.Value, " \t*") {
				values[i] = fmt.Sprintf("%q", v.Value)
			}
			if v.Default {
				values[i] += "*"
			}
		}
		return strings.Join(values, " ")
	}

	var parts []string
	switch fd.Type {
	case workflow.TypeDate:
		parts = append(parts, "YYYY-MM-DD")
	case workflow.TypeRecurrence:
		parts = append(parts, `daily(), weekly("monday"), monthly(1)`)
	case workflow.TypeListRef:
		parts = append(parts, "tiki ids")
	case workflow.TypeUser:
		parts = append(parts, "a name, as a string; user() is the current user")
	}
	if fd.DefaultValue != nil {
		parts = append(parts, "default "+schemaDefault(fd))
	}
	return strings.Join(parts, "; ")
}

func schemaDefault(fd workflow.FieldDef) string {
	if t, ok := fd.DefaultValue.(time.Time); ok {
		if fd.Type == workflow.TypeDate {
			return t.Format("2006-01-02")
		}
		return t.UTC().Format(time.RFC3339)
	}
	b, err := json.Marshal(fd.DefaultValue)
	if err != nil {
		return fmt.Sprint(fd.DefaultValue)
	}
	return string(b)
}

// triggerSchemaRows renders each trigger as when, effect and description.
// A trigger without a description shows its rule instead.
func triggerSchemaRows(defs []config.TriggerDef) ([][]string, error) {
	if len(defs) == 0 {
		return nil, nil
	}
	parser := ruki.NewParser(runtime.NewSchema())
	rows := make([][]string, len(defs))
	for i, def := range defs {
		desc := def.Description
		if desc == "" {
			desc = strings.Join(strings.Fields(def.Ruki), " ")
		}
		rule, err := parser.ParseRule(def.Ruki)
		if err != nil {
			return nil, fmt.Errorf("trigger %q: %w", desc, err)
		}
		when, effect := describeRule(rule)
		rows[i] = []string{when, effect, desc}
	}
	return rows, nil
}

func describeRule(rule *ruki.Rule) (when, effect string) {
	if tt := rule.TimeTrigger; tt != nil {
		return fmt.Sprintf("every %d%s", tt.Interval.Value, tt.Interval.Unit), statementKind(tt.Action)
	}
	tr := rule.Trigger
	when = tr.Timing + " " + tr.Event
	switch {
	case tr.Deny != nil:
		return when, "deny"
	case tr.Run != nil:
		return when, "run"
	default:
		return when, statementKind(tr.Action)
	}
}

func statementKind(stmt *ruki.Statement) string {
	switch {
	case stmt == nil:
		return ""
	case stmt.Create != nil:
		return "create"
	case stmt.Update != nil:
		return "update"
	case stmt.Delete != nil:
		return "delete"
	default:
		return ""
	}
}

func printWorkflowSchemaUsage() {
	fmt.Print(`Usage: tiki workflow schema

Print the fields and triggers of the workflow that applies to the current
directory, in a compact form for AI agents: each field with its type, allowed
values and default, and each trigger with when it fires and what it does.
Views, actions, captions and visuals are left out.

A 'deny' trigger refuses the change it guards. The other triggers run as a
side effect: they update, create or delete tikis, or run a command.
`)
}
