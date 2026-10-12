package view

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/boolean-maybe/tiki/gridlayout"
	"github.com/boolean-maybe/tiki/internal/livestatus"
	"github.com/boolean-maybe/tiki/theme"
)

type fakeLive struct {
	line  livestatus.Line
	ok    bool
	drawn []time.Duration
}

func (f *fakeLive) Line(string) (livestatus.Line, bool) { return f.line, f.ok }
func (f *fakeLive) Drawn(next time.Duration)            { f.drawn = append(f.drawn, next) }

var claudeLine = livestatus.Line{
	Agent:   "✻ Claude",
	Glyph:   "✢",
	Age:     "1m",
	Message: "Implementing",
	Next:    120 * time.Millisecond,
}

func drawOnScreen(t *testing.T, p tview.Primitive, width, height int) tcell.Screen {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen init: %v", err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(width, height)
	p.SetRect(0, 0, width, height)
	p.Draw(screen)
	return screen
}

// Unlike screenRow, it skips the cell a wide glyph spills into, as a terminal would.
func visibleRow(screen tcell.Screen, y int) string {
	width, _ := screen.Size()
	var b strings.Builder
	for x := 0; x < width; x++ {
		str, _, w := screen.Get(x, y)
		if str == "" {
			str = " "
		}
		b.WriteString(str)
		if w == 2 {
			x++
		}
	}
	return b.String()
}

func TestCreateTikiBox_FramedCardShowsTheStatus(t *testing.T) {
	tk := makeTiki("X7F4K2", "story", "medium")
	tk.SetTitle("Status animation")
	spec, err := gridlayout.ParseGrid([][]string{{"id"}, {"title"}})
	if err != nil {
		t.Fatal(err)
	}
	live := &fakeLive{line: claudeLine, ok: true}
	card := CreateTikiBox(tk, spec, false, theme.Roles(), live)
	height := tikiBoxItemHeight(spec)
	screen := drawOnScreen(t, card, 40, height)
	if got, want := visibleRow(screen, height-1), "└─ ✻ Claude ✢ 1m Implementing ─────────┘"; got != want {
		t.Fatalf("bottom row\n got %q\nwant %q", got, want)
	}
}

func TestCreateTikiBox_OneRowCardIgnoresTheStatus(t *testing.T) {
	tk := makeTiki("X7F4K2", "story", "medium")
	tk.SetTitle("Status animation")
	spec, err := gridlayout.ParseGrid([][]string{{"id + \" \" + title"}})
	if err != nil {
		t.Fatal(err)
	}
	live := &fakeLive{line: claudeLine, ok: true}
	screen := drawOnScreen(t, CreateTikiBox(tk, spec, false, theme.Roles(), live), 40, 1)
	if got := visibleRow(screen, 0); strings.Contains(got, "Implementing") {
		t.Fatalf("one-row card shows the status: %q", got)
	}
	if len(live.drawn) != 0 {
		t.Fatal("one-row card reported a drawn status")
	}
}
