package tikidetail

import (
	"strings"
	"testing"

	"github.com/boolean-maybe/tiki/controller"
	"github.com/boolean-maybe/tiki/model"
	"github.com/boolean-maybe/tiki/store"

	"github.com/rivo/tview"
)

const boldBody = "Some **bold** text."

func hasRow(rows []string, sub string) bool {
	i, _ := findRow(rows, sub)
	return i >= 0
}

func sourceLabel(cv *ConfigurableDetailView) string {
	a := cv.GetActionRegistry().GetByID(controller.ActionToggleSource)
	if a == nil {
		return ""
	}
	return a.Label
}

func newSourceModeView(t *testing.T) (*ConfigurableDetailView, store.Store) {
	t.Helper()
	s := store.NewInMemoryStore()
	createTiki(t, s, "AAAAAA", boldBody)
	cv := NewConfigurableDetailView(s, "AAAAAA",
		detailPluginFromFields([]string{"status"}),
		controller.DetailViewActions(), nil, nil, nil, nil)
	return cv, s
}

func TestToggleSourceMode_SwapsRenderedAndSource(t *testing.T) {
	cv, _ := newSourceModeView(t)

	if rows := screenRows(t, cv.GetPrimitive(), 80, 20); !hasRow(rows, "Some bold text.") {
		t.Fatalf("should start rendered:\n%s", strings.Join(rows, "\n"))
	}
	if got := sourceLabel(cv); got != "View source" {
		t.Errorf("label = %q, want %q", got, "View source")
	}

	if !cv.ToggleSourceMode() {
		t.Fatal("toggle refused")
	}
	if rows := screenRows(t, cv.GetPrimitive(), 80, 20); !hasRow(rows, boldBody) {
		t.Errorf("source mode should show the markup:\n%s", strings.Join(rows, "\n"))
	}
	if _, ok := cv.descView.(*tview.TextView); !ok {
		t.Errorf("source mode body is %T, want *tview.TextView", cv.descView)
	}
	if cv.navMarkdown != nil {
		t.Error("source mode should not keep a markdown viewer")
	}
	if got := sourceLabel(cv); got != "View rendered" {
		t.Errorf("label = %q, want %q", got, "View rendered")
	}

	cv.ToggleSourceMode()
	if rows := screenRows(t, cv.GetPrimitive(), 80, 20); !hasRow(rows, "Some bold text.") {
		t.Errorf("second toggle should render again:\n%s", strings.Join(rows, "\n"))
	}
	if got := sourceLabel(cv); got != "View source" {
		t.Errorf("label = %q, want %q", got, "View source")
	}
}

func TestToggleSourceMode_FocusesBodyAndReportsActionChange(t *testing.T) {
	cv, _ := newSourceModeView(t)
	var focused tview.Primitive
	cv.SetFocusSetter(func(p tview.Primitive) { focused = p })
	changes := 0
	cv.SetActionChangeHandler(func() { changes++ })

	cv.ToggleSourceMode()

	if focused == nil || focused != cv.descView {
		t.Error("the new body should take focus")
	}
	if changes != 1 {
		t.Errorf("action change handler fired %d times, want 1", changes)
	}
}

func TestToggleSourceMode_RefusedInEditMode(t *testing.T) {
	cv, _ := newSourceModeView(t)
	cv.editMode = true

	if cv.ToggleSourceMode() {
		t.Error("toggle should be refused while editing")
	}
	if cv.bodyMode.Source() {
		t.Error("mode changed while editing")
	}
}

func TestToggleSourceMode_KeepsFullscreen(t *testing.T) {
	cv, _ := newSourceModeView(t)
	cv.EnterFullscreen()

	cv.ToggleSourceMode()

	if !cv.IsFullscreen() {
		t.Error("toggle left fullscreen")
	}
	if rows := screenRows(t, cv.GetPrimitive(), 80, 20); !hasRow(rows, boldBody) {
		t.Errorf("fullscreen source missing:\n%s", strings.Join(rows, "\n"))
	}
}

func TestSetBodyMode_NewViewOpensInSharedMode(t *testing.T) {
	shared := model.NewDetailBodyMode(false, nil)
	first, s := newSourceModeView(t)
	first.SetBodyMode(shared)
	first.ToggleSourceMode()

	createTiki(t, s, "BBBBBB", "Other *emph* text.")
	second := NewConfigurableDetailView(s, "BBBBBB",
		detailPluginFromFields([]string{"status"}),
		controller.DetailViewActions(), nil, nil, nil, nil)
	second.SetBodyMode(shared)

	if rows := screenRows(t, second.GetPrimitive(), 80, 20); !hasRow(rows, "Other *emph* text.") {
		t.Errorf("second view should open in source:\n%s", strings.Join(rows, "\n"))
	}
	if got := sourceLabel(second); got != "View rendered" {
		t.Errorf("label = %q, want %q", got, "View rendered")
	}
}

func TestSourceMode_StoreChangeStillRebuilds(t *testing.T) {
	cv, s := newSourceModeView(t)
	cv.OnFocus()
	t.Cleanup(cv.OnBlur)
	cv.ToggleSourceMode()

	tk := s.GetTiki("AAAAAA").Clone()
	tk.SetBody("Edited **elsewhere**.")
	if err := s.UpdateTiki(tk); err != nil {
		t.Fatal(err)
	}

	if rows := screenRows(t, cv.GetPrimitive(), 80, 20); !hasRow(rows, "Edited **elsewhere**.") {
		t.Errorf("source view did not pick up the new body:\n%s", strings.Join(rows, "\n"))
	}
}

func TestSourceMode_BlurAndRefreshAreSafe(t *testing.T) {
	cv, _ := newSourceModeView(t)
	cv.OnFocus()
	cv.ToggleSourceMode()
	cv.refresh()
	cv.OnBlur()
	cv.RestoreFocus()
}

func TestSourceMode_LabelSurvivesAnEditRoundTrip(t *testing.T) {
	cv, _ := newSourceModeView(t)
	cv.SetEditModeRegistry(controller.DetailEditModeActions())
	cv.ToggleSourceMode()

	if !cv.EnterEditMode() {
		t.Fatal("could not enter edit mode")
	}
	if cv.GetActionRegistry().ContainsID(controller.ActionToggleSource) {
		t.Error("the toggle must not be offered while editing")
	}
	cv.ExitEditMode()

	if got := sourceLabel(cv); got != "View rendered" {
		t.Errorf("label after editing = %q, want %q", got, "View rendered")
	}
	if rows := screenRows(t, cv.GetPrimitive(), 80, 20); !hasRow(rows, boldBody) {
		t.Errorf("body should still be in source mode:\n%s", strings.Join(rows, "\n"))
	}
}
