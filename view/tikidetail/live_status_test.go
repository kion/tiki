package tikidetail

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/boolean-maybe/tiki/controller"
	"github.com/boolean-maybe/tiki/internal/livestatus"
	"github.com/boolean-maybe/tiki/store"
	"github.com/boolean-maybe/tiki/view/gridbox"
)

type fakeLive struct{ drawn int }

func (f *fakeLive) Line(id string) (livestatus.Line, bool) {
	if id != "SHOWNN" {
		return livestatus.Line{}, false
	}
	return livestatus.Line{Agent: "✻ Claude", Glyph: "✢", Age: "3m", Message: "Implementing"}, true
}

func (f *fakeLive) Drawn(time.Duration) { f.drawn++ }

func TestDetailView_ShowsTheLiveStatusUnderTheMetadata(t *testing.T) {
	s := store.NewInMemoryStore()
	if err := s.CreateTiki(newTestViewTiki("SHOWNN")); err != nil {
		t.Fatal(err)
	}
	plug := detailPluginFromFields([]string{"status", "type"})
	cv := NewConfigurableDetailView(s, "SHOWNN", plug, controller.DetailViewActions(), nil, nil, nil, nil)
	live := &fakeLive{}
	cv.SetLiveStatus(live)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(60, 20)
	root := cv.GetPrimitive()
	root.SetRect(0, 0, 60, 20)
	root.Draw(screen)

	bottom := plug.Layout.Rows + gridbox.DetailBoxOverhead - 1
	want := "└─ ✻ Claude ✢ 3m Implementing ─"
	if got := screenText(screen, bottom, 60); !strings.HasPrefix(got, want) {
		t.Fatalf("metadata box bottom border = %q, want it to start %q", got, want)
	}
	if live.drawn != 1 {
		t.Fatalf("Drawn called %d times, want 1", live.drawn)
	}
}

func screenText(screen tcell.Screen, y, width int) string {
	var b strings.Builder
	for x := 0; x < width; x++ {
		str, _, _ := screen.Get(x, y)
		b.WriteString(str)
	}
	return b.String()
}
