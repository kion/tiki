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
			err := ValidateIDScheme(tt.prefix, "-", tt.defs)
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
	if err := ValidateIDScheme("TIK", "-", defs); err == nil || !strings.Contains(err.Error(), "too long") {
		t.Fatalf("error = %v, want the composed prefix reported as too long", err)
	}
}

func TestIDPrefixForJoinsWithTheSeparator(t *testing.T) {
	if err := RegisterWorkflowFields(schemeFields()); err != nil {
		t.Fatal(err)
	}
	SetProjectPrefix("TIK")
	t.Cleanup(ClearWorkflowFields)
	values := map[string]string{"area": "feature", "layer": "frontend"}
	get := func(field string) string { return values[field] }
	for sep, want := range map[string]string{"-": "FEAT-FE", "_": "FEAT_FE", "": "FEATFE"} {
		SetPrefixSeparator(sep)
		if got := IDPrefixFor(get); got != want {
			t.Errorf("separator %q: IDPrefixFor = %q, want %q", sep, got, want)
		}
	}
}

func TestPrefixSeparatorDefaultsToHyphen(t *testing.T) {
	t.Cleanup(ClearWorkflowFields)
	SetPrefixSeparator("")
	ClearWorkflowFields()
	if got := PrefixSeparator(); got != "-" {
		t.Errorf("PrefixSeparator after ClearWorkflowFields = %q, want -", got)
	}
}

func TestValidateIDSchemeSeparatorRules(t *testing.T) {
	enum := func(name string, prefixes ...string) FieldDef {
		f := FieldDef{Name: name, Type: TypeEnum, Required: true, Immutable: true}
		for i, p := range prefixes {
			f.EnumValues = append(f.EnumValues, EnumValue{Value: name + string(rune('a'+i)), Prefix: p})
		}
		return f
	}
	tests := []struct {
		name, prefix, sep string
		defs              []FieldDef
		wantErr           string
	}{
		{"underscore", "TIK", "_", []FieldDef{enum("t", "B", "W")}, ""},
		{"none", "TIK", "", []FieldDef{enum("t", "B", "W")}, ""},
		{"unknown separator", "TIK", ".", []FieldDef{enum("t", "B")}, `separator "."`},
		{"none needs prefixes ending in a letter", "TIK", "", []FieldDef{enum("t", "B2")}, "end in a letter"},
		{"none needs a project prefix ending in a letter", "T2", "", []FieldDef{enum("t", "B")}, "end in a letter"},
		{"none must not make two combinations alike", "TIK", "", []FieldDef{enum("a", "AB", "A"), enum("b", "C", "BC")}, "both compose"},
		{"hyphen keeps them apart", "TIK", "-", []FieldDef{enum("a", "AB", "A"), enum("b", "C", "BC")}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIDScheme(tt.prefix, tt.sep, tt.defs)
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

func TestSchemeIDsFollowTheSeparator(t *testing.T) {
	defs := []FieldDef{{Name: "type", Type: TypeEnum, Required: true, Immutable: true,
		EnumValues: []EnumValue{{Value: "backlog", Prefix: "B"}, {Value: "watch", Prefix: "W"}}}}
	if err := RegisterWorkflowFields(defs); err != nil {
		t.Fatal(err)
	}
	SetProjectPrefix("TIK")
	SetPrefixSeparator("")
	t.Cleanup(ClearWorkflowFields)

	if got := FormatID("B", 102); got != "B102" {
		t.Errorf("FormatID = %q, want B102", got)
	}
	if p, n, ok := SplitSchemeID("W67"); !ok || p != "W" || n != 67 {
		t.Errorf("SplitSchemeID(W67) = %q, %d, %v", p, n, ok)
	}
	for id, want := range map[string]bool{"B102": true, "W67": true, "TIK3": true, "X95": false, "14SIOZ": false, "B-102": false} {
		if got := MatchesIDScheme(id); got != want {
			t.Errorf("MatchesIDScheme(%q) = %v, want %v", id, got, want)
		}
	}
}
