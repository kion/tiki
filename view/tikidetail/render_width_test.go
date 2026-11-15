package tikidetail

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	nav "github.com/boolean-maybe/navidown/navidown"
	navtview "github.com/boolean-maybe/navidown/navidown/tview"
	"github.com/boolean-maybe/tiki/model"
	"github.com/boolean-maybe/tiki/view/markdown"
)

// steppedRedraw stands in for app.QueueUpdateDraw:
// like it, redraw blocks its caller until the closure has run, which happens when the test steps.
type steppedRedraw struct {
	calls chan func()
}

func newSteppedRedraw() *steppedRedraw { return &steppedRedraw{calls: make(chan func())} }

func (s *steppedRedraw) redraw(fn func()) {
	done := make(chan struct{})
	s.calls <- func() {
		fn()
		close(done)
	}
	<-done
}

// step runs the next closure, or reports that none was queued.
func (s *steppedRedraw) step() bool {
	select {
	case fn := <-s.calls:
		fn()
		return true
	case <-time.After(300 * time.Millisecond):
		return false
	}
}

// asyncDescription starts an off-goroutine render of body into a fresh viewer.
func asyncDescription(t *testing.T, body string) (*ConfigurableDetailView, *markdown.NavigableMarkdown, *steppedRedraw) {
	t.Helper()
	dir := t.TempDir()
	hub := model.NewProgressHub(&progressSink{}, func(fn func()) { fn() })
	t.Cleanup(hub.Stop)

	im := navtview.NewImageManager(nav.NewImageResolver([]string{dir}), 8, 16)
	viewer := markdown.NewNavigableMarkdown(markdown.NavigableMarkdownConfig{
		SearchRoots:  []string{dir},
		ImageManager: im,
	})
	redraw := newSteppedRedraw()
	cv := &ConfigurableDetailView{progressHub: hub, redraw: redraw.redraw}
	cv.imageManager = im
	cv.navMarkdown = viewer
	cv.renderDescription(body, filepath.Join(dir, "doc.md"))
	return cv, viewer, redraw
}

func drawViewer(t *testing.T, viewer *markdown.NavigableMarkdown, width int) {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(width, 30)
	viewer.Viewer().SetRect(0, 0, width, 30)
	viewer.Viewer().Draw(screen)
}

// A refresh replaces the viewer before the first one's render arrives.
// Nobody will ever see the first viewer, so rendering it is wasted work on the UI goroutine.
func TestReplacedViewerIsNotRendered(t *testing.T) {
	cv, first, redraw := asyncDescription(t, "# Title\n\nbody text\n")
	drawViewer(t, first, 100)
	cv.navMarkdown = markdown.NewNavigableMarkdown(markdown.NavigableMarkdownConfig{})

	for redraw.step() {
	}
	if got := first.Viewer().Core().Markdown(); got != "" {
		t.Fatalf("the replaced viewer was rendered with %q", got)
	}
}

// A viewer that has not been drawn has tview's default width,
// where a long body wraps into thousands of rows.
func TestViewerIsRenderedOnlyOnceItHasItsWidth(t *testing.T) {
	_, viewer, redraw := asyncDescription(t, "# Title\n\nbody text\n")

	if !redraw.step() {
		t.Fatal("the render queued nothing for the UI goroutine")
	}
	if got := viewer.Viewer().Core().Markdown(); got != "" {
		t.Fatalf("the viewer was rendered before its first draw, with %q", got)
	}

	// app.QueueUpdateDraw draws after each closure
	drawViewer(t, viewer, 100)
	for redraw.step() {
	}
	if got := viewer.Viewer().Core().Markdown(); got == "" {
		t.Fatal("the viewer was never rendered")
	}
	if got := viewer.Viewer().Core().CurrentWidth(); got != 100 {
		t.Fatalf("rendered at width %d, want the pane's 100", got)
	}
}

func TestDrawnViewerIsRenderedInOneStep(t *testing.T) {
	_, viewer, redraw := asyncDescription(t, "# Title\n\nbody text\n")
	drawViewer(t, viewer, 100)

	if !redraw.step() {
		t.Fatal("the render queued nothing for the UI goroutine")
	}
	if got := viewer.Viewer().Core().Markdown(); got == "" {
		t.Fatal("the drawn viewer was not rendered")
	}
	if redraw.step() {
		t.Fatal("a second closure was queued for a viewer already rendered")
	}
}
