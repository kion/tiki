package app

import (
	"fmt"
	"sync/atomic"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// NewApp creates a tview application.
func NewApp() *tview.Application {
	tview.Borders.HorizontalFocus = tview.Borders.Horizontal
	tview.Borders.VerticalFocus = tview.Borders.Vertical
	tview.Borders.TopLeftFocus = tview.Borders.TopLeft
	tview.Borders.TopRightFocus = tview.Borders.TopRight
	tview.Borders.BottomLeftFocus = tview.Borders.BottomLeft
	tview.Borders.BottomRightFocus = tview.Borders.BottomRight
	return tview.NewApplication()
}

// Run runs the tview application with the given root primitive (typically a tview.Pages).
func Run(app *tview.Application, root tview.Primitive) error {
	app.SetRoot(root, true).EnableMouse(false)
	if err := app.Run(); err != nil {
		return fmt.Errorf("run application: %w", err)
	}
	return nil
}

// InstallTitle sets the terminal title on the next draw, once the screen exists.
// The returned rearm sets it again on the following draw:
// an editor or agent tiki suspended for usually changes the title.
func InstallTitle(application *tview.Application, title string) (rearm func()) {
	var pending atomic.Bool
	pending.Store(true)
	application.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		if pending.CompareAndSwap(true, false) {
			screen.SetTitle(title)
		}
		return false
	})
	return func() { pending.Store(true) }
}
