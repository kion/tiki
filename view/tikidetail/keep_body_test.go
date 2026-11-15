package tikidetail

import (
	"testing"
	"time"

	"github.com/boolean-maybe/tiki/controller"
	"github.com/boolean-maybe/tiki/store"
	tikipkg "github.com/boolean-maybe/tiki/tiki"
)

func focusedViewWithBody(t *testing.T, body string) (*ConfigurableDetailView, *store.InMemoryStore) {
	t.Helper()
	s := store.NewInMemoryStore()
	tk := newTestViewTiki("SHOWNN")
	tk.SetBody(body)
	if err := s.CreateTiki(tk); err != nil {
		t.Fatal(err)
	}
	plug := detailPluginFromFields([]string{"status", "type"})
	cv := NewConfigurableDetailView(s, "SHOWNN", plug, controller.DetailViewActions(), nil, nil, nil, nil)
	cv.OnFocus()
	return cv, s
}

func updateShown(t *testing.T, s *store.InMemoryStore, change func(*tikipkg.Tiki)) {
	t.Helper()
	tk := s.GetTiki("SHOWNN").Clone()
	change(tk)
	if err := s.UpdateTiki(tk); err != nil {
		t.Fatal(err)
	}
}

// An agent's live status replaces the stored tiki with a copy that differs only in its live time.
// Rebuilding the body for that renders the whole markdown again and loses the scroll position.
func TestLiveStatusChangeKeepsTheBodyViewer(t *testing.T) {
	cv, s := focusedViewWithBody(t, "# Title\n\nbody text\n")
	built := cv.navMarkdown

	updateShown(t, s, func(tk *tikipkg.Tiki) { tk.SetLiveAt(time.Now()) })

	if cv.navMarkdown != built {
		t.Fatal("a live status change rebuilt the body viewer")
	}
}

func TestFieldChangeKeepsTheBodyViewerAndUpdatesTheMetadata(t *testing.T) {
	cv, s := focusedViewWithBody(t, "# Title\n\nbody text\n")
	built := cv.navMarkdown
	metadata := cv.content.GetItem(0)

	updateShown(t, s, func(tk *tikipkg.Tiki) { tk.Set("status", "done") })

	if cv.navMarkdown != built {
		t.Fatal("a field change rebuilt the body viewer")
	}
	if cv.content.GetItem(0) == metadata {
		t.Fatal("a field change did not rebuild the metadata box")
	}
	if cv.content.GetItem(1) != built.Viewer() {
		t.Fatal("the kept body viewer is no longer in the layout")
	}
}

func TestBodyChangeRebuildsTheBodyViewer(t *testing.T) {
	cv, s := focusedViewWithBody(t, "# Title\n\nbody text\n")
	built := cv.navMarkdown

	updateShown(t, s, func(tk *tikipkg.Tiki) { tk.SetBody("# Title\n\nnew body text\n") })

	if cv.navMarkdown == built {
		t.Fatal("a body change kept the old body viewer")
	}
	if got := cv.navMarkdown.Viewer().Core().Markdown(); got != "# Title\n\nnew body text\n" {
		t.Fatalf("the body viewer shows %q", got)
	}
}

func TestRenamedWikilinkTargetRebuildsTheBodyViewer(t *testing.T) {
	cv, s := focusedViewWithBody(t, "see [[LINKED]] for background\n")
	linked := tikipkg.New()
	linked.SetID("LINKED")
	linked.SetTitle("old title")
	if err := s.CreateTiki(linked); err != nil {
		t.Fatal(err)
	}
	built := cv.navMarkdown

	renamed := s.GetTiki("LINKED").Clone()
	renamed.SetTitle("new title")
	if err := s.UpdateTiki(renamed); err != nil {
		t.Fatal(err)
	}

	if cv.navMarkdown == built {
		t.Fatal("the body still shows the linked tiki's old title")
	}
}
