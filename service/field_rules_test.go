package service

import (
	"testing"
	"time"

	"github.com/boolean-maybe/tiki/internal/teststatuses"

	tikipkg "github.com/boolean-maybe/tiki/tiki"
	"github.com/boolean-maybe/tiki/workflow"
)

func registerRuleFields(t *testing.T) {
	t.Helper()
	defs := []workflow.FieldDef{
		{
			Name: "area", Type: workflow.TypeEnum, Required: true, Immutable: true,
			EnumValues: []workflow.EnumValue{{Value: "feature", Prefix: "FEAT"}, {Value: "memory", Prefix: "MEM"}, {Value: "misc"}},
		},
		{Name: "owner", Type: workflow.TypeString, Required: true},
		{Name: "origin", Type: workflow.TypeString, Immutable: true},
		{Name: "tags", Type: workflow.TypeListString},
	}
	if err := workflow.RegisterWorkflowFields(defs); err != nil {
		t.Fatal(err)
	}
	workflow.SetProjectPrefix("TIK")
	t.Cleanup(restoreTestFields)
}

// restoreTestFields puts back the canonical fields other tests in this package expect.
func restoreTestFields() {
	workflow.ClearWorkflowFields()
	teststatuses.Init()
}

func ruleTiki(id string, fields map[string]interface{}) *tikipkg.Tiki {
	tk := tikipkg.New()
	tk.SetID(id)
	tk.SetTitle("t")
	for k, v := range fields {
		tk.Set(k, v)
	}
	return tk
}

func reason(r *Rejection) string {
	if r == nil {
		return ""
	}
	return r.Reason
}

func TestRequiredOnCreate(t *testing.T) {
	registerRuleFields(t)
	got := validateRequiredOnCreate(nil, ruleTiki("X7F4K2", map[string]interface{}{"owner": "ann"}), nil)
	if reason(got) != "area is required" {
		t.Errorf("missing area: reason = %q, want %q", reason(got), "area is required")
	}
	ok := ruleTiki("X7F4K2", map[string]interface{}{"area": "misc", "owner": "ann"})
	if r := validateRequiredOnCreate(nil, ok, nil); r != nil {
		t.Errorf("complete tiki rejected: %q", r.Reason)
	}
	empty := ruleTiki("X7F4K2", map[string]interface{}{"area": "misc", "owner": ""})
	if reason(validateRequiredOnCreate(nil, empty, nil)) != "owner is required" {
		t.Errorf("an empty string must not satisfy required")
	}
}

func TestRequiredOnUpdate(t *testing.T) {
	registerRuleFields(t)
	old := ruleTiki("X7F4K2", map[string]interface{}{"area": "misc", "owner": "ann"})
	cleared := ruleTiki("X7F4K2", map[string]interface{}{"area": "misc"})
	if reason(validateRequiredOnUpdate(old, cleared, nil)) != "owner is required" {
		t.Error("clearing a required field must be rejected")
	}
	legacyOld := ruleTiki("X7F4K2", map[string]interface{}{"tags": []string{"a"}})
	legacyNew := ruleTiki("X7F4K2", map[string]interface{}{"tags": []string{"a", "b"}})
	if r := validateRequiredOnUpdate(legacyOld, legacyNew, nil); r != nil {
		t.Errorf("a legacy tiki lacking required fields must stay editable: %q", r.Reason)
	}
}

func TestImmutableOnUpdate(t *testing.T) {
	registerRuleFields(t)
	tests := []struct {
		name       string
		id         string
		old, new   map[string]interface{}
		wantReason string
	}{
		{"changing a set value", "X7F4K2", map[string]interface{}{"origin": "mail"}, map[string]interface{}{"origin": "chat"}, "origin is immutable"},
		{"clearing a set value", "X7F4K2", map[string]interface{}{"origin": "mail"}, map[string]interface{}{}, "origin is immutable"},
		{"unchanged value", "X7F4K2", map[string]interface{}{"origin": "mail"}, map[string]interface{}{"origin": "mail"}, ""},
		{"legacy tiki fills an empty immutable field", "X7F4K2", map[string]interface{}{}, map[string]interface{}{"area": "memory"}, ""},
		{"prefixed tiki fills an empty prefix field", "TIK-4", map[string]interface{}{}, map[string]interface{}{"area": "memory"}, "area is immutable"},
		{"prefixed tiki fills an empty non-prefix immutable field", "TIK-4", map[string]interface{}{}, map[string]interface{}{"origin": "mail"}, ""},
		{"prefix field changed on a prefixed tiki", "FEAT-2", map[string]interface{}{"area": "feature"}, map[string]interface{}{"area": "memory"}, "area is immutable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateImmutableOnUpdate(ruleTiki(tt.id, tt.old), ruleTiki(tt.id, tt.new), nil)
			if reason(got) != tt.wantReason {
				t.Errorf("reason = %q, want %q", reason(got), tt.wantReason)
			}
		})
	}
}

func TestFieldRulesIgnoreWorkflowsWithoutThem(t *testing.T) {
	defs := []workflow.FieldDef{{Name: "status", Type: workflow.TypeEnum, EnumValues: []workflow.EnumValue{{Value: "open"}, {Value: "done"}}}}
	if err := workflow.RegisterWorkflowFields(defs); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restoreTestFields)
	old := ruleTiki("X7F4K2", map[string]interface{}{"status": "open"})
	new := ruleTiki("X7F4K2", map[string]interface{}{})
	if r := validateImmutableOnUpdate(old, new, nil); r != nil {
		t.Errorf("immutable rejection without immutable fields: %q", r.Reason)
	}
	if r := validateRequiredOnCreate(nil, new, nil); r != nil {
		t.Errorf("required rejection without required fields: %q", r.Reason)
	}
}

func TestFieldRulesRegisteredOnGate(t *testing.T) {
	registerRuleFields(t)
	g := NewTikiMutationGate()
	RegisterFieldValidators(g)
	if err := g.runValidators(g.createValidators, nil, ruleTiki("X7F4K2", map[string]interface{}{"owner": "ann"}), nil); err == nil {
		t.Error("create without a required field passed the gate's validators")
	}
	old := ruleTiki("X7F4K2", map[string]interface{}{"area": "misc", "owner": "ann", "origin": "mail"})
	new := ruleTiki("X7F4K2", map[string]interface{}{"area": "misc", "owner": "ann", "origin": "chat"})
	if err := g.runValidators(g.updateValidators, old, new, nil); err == nil {
		t.Error("changing an immutable field passed the gate's validators")
	}
}

func TestImmutableTreatsTheSameInstantAsUnchanged(t *testing.T) {
	defs := []workflow.FieldDef{{Name: "due", Type: workflow.TypeDate, Immutable: true}}
	if err := workflow.RegisterWorkflowFields(defs); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restoreTestFields)
	utc := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	local := utc.In(time.FixedZone("X", 3600))
	old := ruleTiki("X7F4K2", map[string]interface{}{"due": utc})
	new := ruleTiki("X7F4K2", map[string]interface{}{"due": local})
	if r := validateImmutableOnUpdate(old, new, nil); r != nil {
		t.Errorf("same instant rejected: %q", r.Reason)
	}
}

func TestImmutableListComparesElements(t *testing.T) {
	defs := []workflow.FieldDef{{Name: "labels", Type: workflow.TypeListString, Immutable: true}}
	if err := workflow.RegisterWorkflowFields(defs); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restoreTestFields)
	old := ruleTiki("X7F4K2", map[string]interface{}{"labels": []string{"a b"}})
	split := ruleTiki("X7F4K2", map[string]interface{}{"labels": []string{"a", "b"}})
	if r := validateImmutableOnUpdate(old, split, nil); r == nil {
		t.Error(`["a b"] → ["a","b"] passed as unchanged`)
	}
	same := ruleTiki("X7F4K2", map[string]interface{}{"labels": []interface{}{"a b"}})
	if r := validateImmutableOnUpdate(old, same, nil); r != nil {
		t.Errorf("the same list in another Go type was rejected: %q", r.Reason)
	}
}
