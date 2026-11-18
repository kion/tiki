package workflow

import (
	"strings"
	"testing"
)

func schemeFields() []FieldDef {
	return []FieldDef{
		{
			Name: "area", Type: TypeEnum, Required: true, Immutable: true,
			EnumValues: []EnumValue{
				{Value: "feature", Prefix: "FEAT"},
				{Value: "memory", Prefix: "MEM"},
				{Value: "misc"},
			},
		},
		{Name: "status", Type: TypeEnum, EnumValues: []EnumValue{{Value: "open"}, {Value: "done"}}},
		{
			Name: "layer", Type: TypeEnum, Required: true, Immutable: true,
			EnumValues: []EnumValue{
				{Value: "frontend", Prefix: "FE"},
				{Value: "database", Prefix: "DB"},
				{Value: "cross"},
			},
		},
	}
}

func TestValidateIDScheme(t *testing.T) {
	withArea := func(mutate func(*FieldDef)) []FieldDef {
		defs := schemeFields()
		mutate(&defs[0])
		return defs
	}
	tests := []struct {
		name    string
		prefix  string
		defs    []FieldDef
		wantErr string
	}{
		{"valid scheme", "TIK", schemeFields(), ""},
		{"project prefix alone", "TIK", []FieldDef{{Name: "status", Type: TypeEnum, EnumValues: []EnumValue{{Value: "open"}}}}, ""},
		{"no prefixes at all", "", []FieldDef{{Name: "status", Type: TypeEnum, EnumValues: []EnumValue{{Value: "open"}}}}, ""},
		{"value prefixes need a project prefix", "", schemeFields(), "needs a top-level prefix"},
		{"lowercase project prefix", "tik", schemeFields(), `top-level prefix "tik"`},
		{"lowercase value prefix", "TIK", withArea(func(f *FieldDef) { f.EnumValues[0].Prefix = "feat" }), `prefix "feat"`},
		{"prefix field must be required", "TIK", withArea(func(f *FieldDef) { f.Required = false }), "required: true and immutable: true"},
		{"prefix field must be immutable", "TIK", withArea(func(f *FieldDef) { f.Immutable = false }), "required: true and immutable: true"},
		{"prefix used twice", "TIK", withArea(func(f *FieldDef) { f.EnumValues[1].Prefix = "FE" }), `prefix "FE" is used twice`},
		{"value prefix equals project prefix", "TIK", withArea(func(f *FieldDef) { f.EnumValues[1].Prefix = "TIK" }), "same as the top-level prefix"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIDScheme(tt.prefix, tt.defs)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestIDPrefixFor(t *testing.T) {
	if err := RegisterWorkflowFields(schemeFields()); err != nil {
		t.Fatal(err)
	}
	SetProjectPrefix("TIK")
	t.Cleanup(ClearWorkflowFields)

	tests := []struct {
		area, layer string
		want        string
	}{
		{"feature", "frontend", "FEAT-FE"},
		{"feature", "cross", "FEAT"},
		{"misc", "database", "DB"},
		{"misc", "cross", "TIK"},
		{"", "frontend", "FE"},
		{"", "", "TIK"},
	}
	for _, tt := range tests {
		values := map[string]string{"area": tt.area, "layer": tt.layer, "status": "open"}
		got := IDPrefixFor(func(field string) string { return values[field] })
		if got != tt.want {
			t.Errorf("area=%q layer=%q: IDPrefixFor = %q, want %q", tt.area, tt.layer, got, tt.want)
		}
	}
}

func TestPrefixedIDsEnabledFollowsProjectPrefix(t *testing.T) {
	t.Cleanup(ClearWorkflowFields)
	ClearWorkflowFields()
	if PrefixedIDsEnabled() {
		t.Fatal("enabled without a project prefix")
	}
	SetProjectPrefix("TIK")
	if !PrefixedIDsEnabled() || ProjectPrefix() != "TIK" {
		t.Fatalf("PrefixedIDsEnabled=%v ProjectPrefix=%q", PrefixedIDsEnabled(), ProjectPrefix())
	}
	ClearWorkflowFields()
	if PrefixedIDsEnabled() {
		t.Fatal("ClearWorkflowFields must clear the project prefix")
	}
}

func TestPrefixFieldsInDeclarationOrder(t *testing.T) {
	if err := RegisterWorkflowFields(schemeFields()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ClearWorkflowFields)
	got := PrefixFields()
	if len(got) != 2 || got[0].Name != "area" || got[1].Name != "layer" {
		t.Fatalf("PrefixFields = %v, want area, layer", got)
	}
}

func TestValidateIDSchemeBoundsTheComposedPrefix(t *testing.T) {
	long := strings.Repeat("A", 30)
	defs := []FieldDef{
		{Name: "a", Type: TypeEnum, Required: true, Immutable: true, EnumValues: []EnumValue{{Value: "x", Prefix: long}}},
		{Name: "b", Type: TypeEnum, Required: true, Immutable: true, EnumValues: []EnumValue{{Value: "y", Prefix: "B" + long}}},
	}
	if err := ValidateIDScheme("TIK", defs); err == nil || !strings.Contains(err.Error(), "too long") {
		t.Fatalf("error = %v, want the composed prefix reported as too long", err)
	}
}
