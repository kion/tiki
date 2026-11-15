package tikidetail

import (
	"github.com/rivo/tview"

	"testing"

	"github.com/boolean-maybe/tiki/controller"
	"github.com/boolean-maybe/tiki/model"
	"github.com/boolean-maybe/tiki/store"
)

// openedLikeTheFactory repeats what the view factory and the root layout do to a new detail view.
func openedLikeTheFactory(t *testing.T) *ConfigurableDetailView {
	t.Helper()
	s := store.NewInMemoryStore()
	if err := s.CreateTiki(newTestViewTiki("SHOWNN")); err != nil {
		t.Fatal(err)
	}
	plug := detailPluginFromFields([]string{"status", "type"})
	return NewConfigurableDetailView(s, "SHOWNN", plug, controller.DetailViewActions(), nil, nil, nil, nil)
}

// Every rebuild makes a new body viewer and renders the whole body into it,
// so the steps of opening a view must not each rebuild it.
func TestOpeningADetailViewBuildsItsBodyOnce(t *testing.T) {
	cv := openedLikeTheFactory(t)
	built := cv.navMarkdown
	if built == nil {
		t.Fatal("the constructor built no body viewer")
	}

	cv.SetLiveStatus(&fakeLive{})
	if cv.navMarkdown != built {
		t.Fatal("setting the live status source rebuilt the body")
	}
	cv.SetBodyMode(model.NewDetailBodyMode(false, nil))
	if cv.navMarkdown != built {
		t.Fatal("installing the shared body mode rebuilt the body")
	}
	cv.OnFocus()
	if cv.navMarkdown != built {
		t.Fatal("the first focus rebuilt the body")
	}
}

func TestFocusAfterBlurRebuildsTheBody(t *testing.T) {
	cv := openedLikeTheFactory(t)
	cv.OnFocus()
	cv.OnBlur()
	if cv.navMarkdown != nil {
		t.Fatal("blur kept the body viewer")
	}

	cv.OnFocus()
	if cv.navMarkdown == nil {
		t.Fatal("focus after a blur did not rebuild the body")
	}
}

func TestFocusFocusesTheBodyWithoutRebuildingIt(t *testing.T) {
	cv := openedLikeTheFactory(t)
	var focused any
	cv.SetFocusSetter(func(p tview.Primitive) { focused = p })

	cv.OnFocus()
	if focused != any(cv.descView) {
		t.Fatalf("focus went to %T, want the body viewer", focused)
	}
}
