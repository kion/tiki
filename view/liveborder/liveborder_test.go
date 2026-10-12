package liveborder

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/boolean-maybe/tiki/internal/livestatus"
	"github.com/boolean-maybe/tiki/theme"
)

func init() {
	theme.SetTheme(theme.LoadByName("dark"))
}

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

const boxHeight = 4

func newTestFrame(live Source) *Frame {
	frame := tview.NewFrame(tview.NewBox()).SetBorders(0, 0, 0, 0, 0, 3)
	frame.SetBorder(true)
	return New(frame, "X7F4K2", live, theme.Roles())
}

func drawOnScreen(t *testing.T, p tview.Primitive, width int) tcell.Screen {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen init: %v", err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(width, boxHeight)
	p.SetRect(0, 0, width, boxHeight)
	p.Draw(screen)
	return screen
}

// It skips the cell a wide glyph spills into, as a terminal would.
func bottomRow(screen tcell.Screen) string {
	width, _ := screen.Size()
	var b strings.Builder
	for x := 0; x < width; x++ {
		str, _, w := screen.Get(x, boxHeight-1)
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

func borderForegroundAt(screen tcell.Screen, x int) tcell.Color {
	_, style, _ := screen.Get(x, boxHeight-1)
	fg, _, _ := style.Decompose()
	return fg
}

func TestFrame_PaintsTheStatusIntoTheBottomBorder(t *testing.T) {
	live := &fakeLive{line: claudeLine, ok: true}
	screen := drawOnScreen(t, newTestFrame(live), 40)

	want := "└─ ✻ Claude ✢ 1m Implementing ─────────┘"
	if got := bottomRow(screen); got != want {
		t.Fatalf("bottom row\n got %q\nwant %q", got, want)
	}
	if len(live.drawn) != 1 || live.drawn[0] != 120*time.Millisecond {
		t.Fatalf("Drawn calls = %v, want [120ms]", live.drawn)
	}
}

func TestFrame_NoAgeLeavesNoGap(t *testing.T) {
	line := claudeLine
	line.Age = ""
	screen := drawOnScreen(t, newTestFrame(&fakeLive{line: line, ok: true}), 40)
	want := "└─ ✻ Claude ✢ Implementing ────────────┘"
	if got := bottomRow(screen); got != want {
		t.Fatalf("bottom row\n got %q\nwant %q", got, want)
	}
}

func TestFrame_DropsTheAgentBeforeTheMessage(t *testing.T) {
	screen := drawOnScreen(t, newTestFrame(&fakeLive{line: claudeLine, ok: true}), 20)
	want := "└─ ✢ 1m Implemen… ─┘"
	if got := bottomRow(screen); got != want {
		t.Fatalf("bottom row\n got %q\nwant %q", got, want)
	}
}

func TestFrame_DropsTheAgeBeforeTheMessage(t *testing.T) {
	frame := newTestFrame(&fakeLive{line: claudeLine, ok: true})
	frame.SetBottomBorderLimit(11)
	screen := drawOnScreen(t, frame, 40)
	want := "└─ ✢ Impl… ────────────────────────────┘"
	if got := bottomRow(screen); got != want {
		t.Fatalf("bottom row\n got %q\nwant %q", got, want)
	}
}

func TestFrame_GlyphAloneWhenTheMessageCannotFit(t *testing.T) {
	frame := newTestFrame(&fakeLive{line: claudeLine, ok: true})
	frame.SetBottomBorderLimit(7)
	screen := drawOnScreen(t, frame, 20)
	if got, want := bottomRow(screen), "└─ ✢ ──────────────┘"; got != want {
		t.Fatalf("bottom row\n got %q\nwant %q", got, want)
	}
}

func TestFrame_SkipsAMissingAgentAndGlyph(t *testing.T) {
	line := livestatus.Line{Age: "3m", Message: "Testing"}
	screen := drawOnScreen(t, newTestFrame(&fakeLive{line: line, ok: true}), 30)
	want := "└─ 3m Testing ───────────────┘"
	if got := bottomRow(screen); got != want {
		t.Fatalf("bottom row\n got %q\nwant %q", got, want)
	}
}

func TestFrame_WideGlyph(t *testing.T) {
	line := claudeLine
	line.Agent = ""
	line.Glyph = "🌑"
	screen := drawOnScreen(t, newTestFrame(&fakeLive{line: line, ok: true}), 30)
	want := "└─ 🌑 1m Implementing ───────┘"
	if got := bottomRow(screen); got != want {
		t.Fatalf("bottom row\n got %q\nwant %q", got, want)
	}
}

func TestFrame_NoStatusLeavesTheBorderAlone(t *testing.T) {
	live := &fakeLive{}
	screen := drawOnScreen(t, newTestFrame(live), 20)
	if got := bottomRow(screen); got != "└──────────────────┘" {
		t.Fatalf("bottom row = %q", got)
	}
	if len(live.drawn) != 0 {
		t.Fatalf("Drawn called %d times for a box without a status", len(live.drawn))
	}
}

func TestFrame_NilSourceLeavesTheBorderAlone(t *testing.T) {
	screen := drawOnScreen(t, newTestFrame(nil), 20)
	if got := bottomRow(screen); got != "└──────────────────┘" {
		t.Fatalf("bottom row = %q", got)
	}
}

func TestFrame_Colors(t *testing.T) {
	roles := theme.Roles()
	agentX, glyphX, ageX, messageX := 3, 12, 14, 22

	fresh := drawOnScreen(t, newTestFrame(&fakeLive{line: claudeLine, ok: true}), 40)
	for name, c := range map[string]struct {
		x    int
		want theme.Role
	}{
		"agent":   {agentX, roles.LiveAgent()},
		"glyph":   {glyphX, roles.LiveAgent()},
		"age":     {ageX, roles.TextMuted()},
		"message": {messageX, roles.LiveMessage()},
	} {
		if got := borderForegroundAt(fresh, c.x); got != c.want.TCell() {
			t.Errorf("%s fg = %v, want %v", name, got, c.want.TCell())
		}
	}

	staleLine := claudeLine
	staleLine.Stale = true
	stale := drawOnScreen(t, newTestFrame(&fakeLive{line: staleLine, ok: true}), 40)
	for _, x := range []int{agentX, glyphX, ageX, messageX} {
		if got, want := borderForegroundAt(stale, x), roles.TextMuted().TCell(); got != want {
			t.Errorf("stale fg at %d = %v, want muted %v", x, got, want)
		}
	}
}

func TestFrame_KeepsTheBorderBackground(t *testing.T) {
	plainFrame := newTestFrame(nil)
	plainFrame.SetBackgroundColor(tcell.NewRGBColor(10, 20, 30))
	plain := drawOnScreen(t, plainFrame, 40)
	_, plainStyle, _ := plain.Get(5, boxHeight-1)
	_, wantBg, _ := plainStyle.Decompose()

	liveFrame := newTestFrame(&fakeLive{line: claudeLine, ok: true})
	liveFrame.SetBackgroundColor(tcell.NewRGBColor(10, 20, 30))
	painted := drawOnScreen(t, liveFrame, 40)
	_, style, _ := painted.Get(5, boxHeight-1)
	if _, bg, _ := style.Decompose(); bg != wantBg {
		t.Fatalf("background = %v, want the border's %v", bg, wantBg)
	}
}
