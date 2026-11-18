package tikidetail

import (
	"testing"

	"github.com/boolean-maybe/tiki/controller"
	"github.com/boolean-maybe/tiki/internal/teststatuses"
	"github.com/boolean-maybe/tiki/store"
	tikipkg "github.com/boolean-maybe/tiki/tiki"
	"github.com/boolean-maybe/tiki/workflow"
)

func registerImmutableFields(t *testing.T) {
	t.Helper()
	extra := []workflow.FieldDef{
		{
			Name: "area", Type: workflow.TypeEnum, Required: true, Immutable: true,
			EnumValues: []workflow.EnumValue{{Value: "feature", Prefix: "FEAT"}, {Value: "misc"}},
		},
		{
			Name: "origin", Type: workflow.TypeEnum, Immutable: true,
			EnumValues: []workflow.EnumValue{{Value: "mail"}, {Value: "chat"}},
		},
	}
	if err := teststatuses.InitWith(extra); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		workflow.ClearWorkflowFields()
		teststatuses.Init()
	})
}

func immutableView(t *testing.T, s store.Store, id string) *ConfigurableDetailView {
	t.Helper()
	return NewConfigurableDetailView(
		s, id, detailPluginFromFields([]string{"status", "area", "origin"}),
		controller.DetailViewActions(), nil, nil, nil, nil,
	)
}

func storedTiki(t *testing.T, s store.Store, id string, fields map[string]string) {
	t.Helper()
	tk := newTestViewTiki(id)
	for k, v := range fields {
		tk.Set(k, v)
	}
	if err := s.CreateTiki(tk); err != nil {
		t.Fatalf("CreateTiki(%s): %v", id, err)
	}
}

func TestImmutableFieldsAreReadOnlyOnceSet(t *testing.T) {
	registerImmutableFields(t)
	s := store.NewInMemoryStore()
	storedTiki(t, s, "AAAAAA", map[string]string{"origin": "mail"})
	storedTiki(t, s, "BBBBBB", map[string]string{})
	storedTiki(t, s, "TIK-4", map[string]string{})

	tests := []struct {
		id, field string
		want      bool
	}{
		{"AAAAAA", "origin", false}, // set: immutable
		{"AAAAAA", "status", true},  // ordinary field stays editable
		{"BBBBBB", "origin", true},  // legacy tiki may fill it once
		{"BBBBBB", "area", true},    // legacy tiki may fill a prefix field once
		{"TIK-4", "area", false},    // the id already carries the prefix
		{"TIK-4", "origin", true},   // not a prefix field: fill once
	}
	for _, tt := range tests {
		cv := immutableView(t, s, tt.id)
		if got := cv.isEditableLayoutField(tt.field); got != tt.want {
			t.Errorf("%s.%s editable = %v, want %v", tt.id, tt.field, got, tt.want)
		}
	}
}

func TestDraftKeepsImmutableFieldsEditable(t *testing.T) {
	registerImmutableFields(t)
	s := store.NewInMemoryStore()
	draft := tikipkg.New()
	draft.SetID("CCCCCC")
	draft.SetTitle("draft")
	draft.Set("area", "feature")
	draft.Set("origin", "chat")
	cv := immutableView(t, s, draft.ID())
	cv.SetFallbackTiki(draft)
	for _, f := range []string{"area", "origin"} {
		if !cv.isEditableLayoutField(f) {
			t.Errorf("draft field %s must stay editable", f)
		}
	}
}
