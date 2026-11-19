package controller

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestSuspend_CallsAfterResume(t *testing.T) {
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	application := tview.NewApplication()
	application.SetScreen(sim)
	nc := NewNavigationController(application)
	nc.SetCommandRunner(func(string, ...string) error { return nil })
	nc.SetEditorOpener(func(string) error { return nil })
	resumed := 0
	nc.SetAfterResume(func() { resumed++ })

	nc.SuspendAndRun("agent")
	_ = nc.SuspendAndEdit("file.md")

	if resumed != 2 {
		t.Errorf("after-resume hook ran %d times, want 2", resumed)
	}
}
