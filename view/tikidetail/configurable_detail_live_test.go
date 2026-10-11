package tikidetail

import (
	"testing"
	"time"

	"github.com/boolean-maybe/tiki/controller"
	"github.com/boolean-maybe/tiki/store"
	tikipkg "github.com/boolean-maybe/tiki/tiki"
)

type liveDetailFixture struct {
	store *store.InMemoryStore
	view  *ConfigurableDetailView
}

func newLiveDetailFixture(t *testing.T) *liveDetailFixture {
	t.Helper()
	s := store.NewInMemoryStore()
	linked := tikipkg.New()
	linked.SetID("LINKED")
	linked.SetTitle("linked tiki")
	other := tikipkg.New()
	other.SetID("OTHERS")
	other.SetTitle("unrelated tiki")
	shown := newTestViewTiki("SHOWNN")
	shown.Set("dependsOn", []string{"LINKED"})
	shown.LoadedMtime = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tk := range []*tikipkg.Tiki{linked, other, shown} {
		if err := s.CreateTiki(tk); err != nil {
			t.Fatalf("CreateTiki(%s): %v", tk.ID(), err)
		}
	}
	cv := NewConfigurableDetailView(s, "SHOWNN",
		detailPluginFromFields([]string{"status", "type", "dependsOn"}),
		controller.DetailViewActions(), nil, nil, nil, nil)
	cv.OnFocus()
	t.Cleanup(cv.OnBlur)
	return &liveDetailFixture{store: s, view: cv}
}

// A reload replaces tikis with new objects.
func (f *liveDetailFixture) replace(t *testing.T, id string, mutate func(*tikipkg.Tiki)) {
	t.Helper()
	tk := f.store.GetTiki(id).Clone()
	mutate(tk)
	if err := f.store.UpdateTiki(tk); err != nil {
		t.Fatalf("UpdateTiki(%s): %v", id, err)
	}
}

func TestDetailView_UnrelatedStoreChangeKeepsRender(t *testing.T) {
	f := newLiveDetailFixture(t)
	before := f.view.descView

	f.replace(t, "OTHERS", func(tk *tikipkg.Tiki) { tk.SetTitle("changed elsewhere") })

	if f.view.descView != before {
		t.Error("an unrelated tiki's change rebuilt the detail view (scroll and focus lost)")
	}
}

func TestDetailView_ShownTikiChangeRebuilds(t *testing.T) {
	f := newLiveDetailFixture(t)
	before := f.view.descView

	f.replace(t, "SHOWNN", func(tk *tikipkg.Tiki) { tk.SetTitle("edited elsewhere") })

	if f.view.descView == before {
		t.Error("a change to the shown tiki did not re-render it")
	}
}

func TestDetailView_LinkedTikiChangeRebuilds(t *testing.T) {
	f := newLiveDetailFixture(t)
	before := f.view.descView

	f.replace(t, "LINKED", func(tk *tikipkg.Tiki) { tk.SetTitle("renamed dependency") })

	if f.view.descView == before {
		t.Error("a linked tiki's title change did not re-render the dependency list")
	}
}

func TestDetailView_ExternalChangeWhileEditingNotifiesOnce(t *testing.T) {
	f := newLiveDetailFixture(t)
	editing := f.store.GetTiki("SHOWNN").Clone()
	f.view.SetEditTikiSource(func() *tikipkg.Tiki { return editing })
	calls := 0
	f.view.SetExternalChangeHandler(func() { calls++ })
	if !f.view.EnterEditMode() {
		t.Fatal("EnterEditMode failed")
	}
	editorsBefore := len(f.view.editors)
	descBefore := f.view.descView

	f.replace(t, "SHOWNN", func(tk *tikipkg.Tiki) {
		tk.SetTitle("edited elsewhere")
		tk.LoadedMtime = tk.LoadedMtime.Add(time.Minute)
	})
	if calls != 1 {
		t.Fatalf("external change handler called %d times, want 1", calls)
	}
	if f.view.descView != descBefore || len(f.view.editors) != editorsBefore {
		t.Error("external change rebuilt the editors mid-edit")
	}

	f.replace(t, "OTHERS", func(tk *tikipkg.Tiki) { tk.SetTitle("noise") })
	if calls != 1 {
		t.Errorf("handler called again for an unrelated change: %d calls", calls)
	}
}

func TestDetailView_ExternalDeleteWhileEditingNotifies(t *testing.T) {
	f := newLiveDetailFixture(t)
	editing := f.store.GetTiki("SHOWNN").Clone()
	f.view.SetEditTikiSource(func() *tikipkg.Tiki { return editing })
	calls := 0
	f.view.SetExternalChangeHandler(func() { calls++ })
	if !f.view.EnterEditMode() {
		t.Fatal("EnterEditMode failed")
	}

	f.store.DeleteTiki("SHOWNN")

	if calls != 1 {
		t.Errorf("external delete: handler called %d times, want 1", calls)
	}
}

func TestDetailView_NoExternalChangeReportOutsideEditMode(t *testing.T) {
	f := newLiveDetailFixture(t)
	calls := 0
	f.view.SetExternalChangeHandler(func() { calls++ })

	f.replace(t, "SHOWNN", func(tk *tikipkg.Tiki) {
		tk.LoadedMtime = tk.LoadedMtime.Add(time.Minute)
	})

	if calls != 0 {
		t.Errorf("handler called %d times outside edit mode", calls)
	}
}

func TestDetailView_ExternalChangeWithSameMtimeNotifies(t *testing.T) {
	f := newLiveDetailFixture(t)
	editing := f.store.GetTiki("SHOWNN").Clone()
	editing.LoadedSize = 100
	f.view.SetEditTikiSource(func() *tikipkg.Tiki { return editing })
	calls := 0
	f.view.SetExternalChangeHandler(func() { calls++ })
	if !f.view.EnterEditMode() {
		t.Fatal("EnterEditMode failed")
	}

	// coarse timestamps: the competing write kept the mtime
	f.replace(t, "SHOWNN", func(tk *tikipkg.Tiki) { tk.LoadedSize = 140 })

	if calls != 1 {
		t.Errorf("external change with an unchanged mtime: handler called %d times, want 1", calls)
	}
}
