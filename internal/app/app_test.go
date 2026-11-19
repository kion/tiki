package app

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type titleScreen struct {
	tcell.Screen
	titles []string
}

func (s *titleScreen) SetTitle(title string) { s.titles = append(s.titles, title) }

func TestInstallTitle_SetsOnFirstDrawAndAfterRearm(t *testing.T) {
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	screen := &titleScreen{Screen: sim}
	application := tview.NewApplication()
	application.SetScreen(screen)
	application.SetRoot(tview.NewBox(), true)

	rearm := InstallTitle(application, "Acme • tiki")
	application.ForceDraw()
	application.ForceDraw()
	if len(screen.titles) != 1 || screen.titles[0] != "Acme • tiki" {
		t.Fatalf("titles after two draws = %q, want one", screen.titles)
	}

	rearm()
	application.ForceDraw()
	if len(screen.titles) != 2 {
		t.Errorf("titles after rearm = %q, want it set again", screen.titles)
	}
}
