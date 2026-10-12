package view

import (
	"strings"
	"testing"

	gradcore "github.com/boolean-maybe/tiki/internal/gradient"
	"github.com/boolean-maybe/tiki/theme"
	"github.com/gdamore/tcell/v2"
)

// fakePositionPaint returns a distinct color at t=0, t=1, and any other t.
// Used to verify which branch of bgColorAt fired without depending on a
// real theme or screen.
type fakePositionPaint struct{}

func (fakePositionPaint) ColorAt(t float64) tcell.Color {
	switch {
	case t == 0.0:
		return tcell.NewRGBColor(10, 10, 10)
	case t == 1.0:
		return tcell.NewRGBColor(200, 200, 200)
	default:
		return tcell.NewRGBColor(100, 100, 100)
	}
}

// TestGradientCaptionRow_BgColorAt_CapabilityBranches verifies the three-way
// capability switch restored after the role/modifier migration: truecolor
// passes t through, 256-color flattens to t=0, 8/16-color flattens to t=1.
// Regression coverage for the bug where Tiki 7 deleted UseWideGradients and
// caused 256-color banding plus 8/16-color invisible captions.
func TestGradientCaptionRow_BgColorAt_CapabilityBranches(t *testing.T) {
	gcr := &GradientCaptionRow{paint: fakePositionPaint{}}

	// save and restore capability flags around the test
	prevWide := gradcore.UseWideGradients.Load()
	prevAny := gradcore.UseGradients.Load()
	defer gradcore.UseWideGradients.Store(prevWide)
	defer gradcore.UseGradients.Store(prevAny)

	cases := []struct {
		name      string
		wide, any bool
		t         float64
		want      tcell.Color
	}{
		{"truecolor passes t through (mid)", true, true, 0.5, tcell.NewRGBColor(100, 100, 100)},
		{"truecolor passes t through (zero)", true, true, 0.0, tcell.NewRGBColor(10, 10, 10)},
		{"256-color flattens to gradient start", false, true, 0.5, tcell.NewRGBColor(10, 10, 10)},
		{"8/16-color flattens to gradient end", false, false, 0.5, tcell.NewRGBColor(200, 200, 200)},
		// UseWideGradients=true but UseGradients=false should never happen in
		// practice (bootstrap clears wide when any is false), but defend
		// against drift by asserting the wide path still runs.
		{"wide overrides any (defensive)", true, false, 0.5, tcell.NewRGBColor(100, 100, 100)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gradcore.UseWideGradients.Store(c.wide)
			gradcore.UseGradients.Store(c.any)
			got := gcr.bgColorAt(c.t)
			if got != c.want {
				t.Errorf("bgColorAt(%v) = %v, want %v", c.t, got, c.want)
			}
		})
	}
}

func TestComputeLaneBoundaries(t *testing.T) {
	tests := []struct {
		name       string
		weights    []int
		numLanes   int
		totalWidth int
		wantStarts []int
		wantEnds   []int
	}{
		{
			"equal weights",
			[]int{1, 1, 1}, 3, 90,
			[]int{0, 30, 60}, []int{30, 60, 90},
		},
		{
			"nil weights",
			nil, 3, 90,
			[]int{0, 30, 60}, []int{30, 60, 90},
		},
		{
			"proportional 25-50-25",
			[]int{25, 50, 25}, 3, 100,
			[]int{0, 25, 75}, []int{25, 75, 100},
		},
		{
			"single lane",
			[]int{1}, 1, 80,
			[]int{0}, []int{80},
		},
		{
			"last lane absorbs rounding",
			[]int{1, 1, 1}, 3, 100,
			// 100/3=33, 66, last gets 100
			[]int{0, 33, 66}, []int{33, 66, 100},
		},
		{
			"two lanes unequal",
			[]int{1, 3}, 2, 80,
			[]int{0, 20}, []int{20, 80},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			starts, ends := computeLaneBoundaries(tt.weights, tt.numLanes, tt.totalWidth)
			if len(starts) != tt.numLanes || len(ends) != tt.numLanes {
				t.Fatalf("len(starts)=%d, len(ends)=%d, want %d", len(starts), len(ends), tt.numLanes)
			}
			for i := range starts {
				if starts[i] != tt.wantStarts[i] {
					t.Errorf("starts[%d] = %d, want %d (full: %v)", i, starts[i], tt.wantStarts[i], starts)
					break
				}
			}
			for i := range ends {
				if ends[i] != tt.wantEnds[i] {
					t.Errorf("ends[%d] = %d, want %d (full: %v)", i, ends[i], tt.wantEnds[i], ends)
					break
				}
			}
			// verify contiguity: each start == previous end
			for i := 1; i < tt.numLanes; i++ {
				if starts[i] != ends[i-1] {
					t.Errorf("gap between lane %d and %d: end=%d, start=%d", i-1, i, ends[i-1], starts[i])
				}
			}
			// last end == totalWidth
			if ends[tt.numLanes-1] != tt.totalWidth {
				t.Errorf("last end = %d, want %d", ends[tt.numLanes-1], tt.totalWidth)
			}
		})
	}
}

func TestCaptionLabel(t *testing.T) {
	cases := []struct {
		name  string
		lane  string
		count int
		width int
		want  string
	}{
		{"count follows the name", "Ready", 12, 20, "Ready · 12"},
		{"exact fit", "Ready", 12, 10, "Ready · 12"},
		{"narrow lane shortens the name, not the count", "In Progress", 12, 10, "In P… · 12"},
		{"no room for a name keeps the count", "Ready", 12, 6, "12"},
		{"empty lane shows zero", "Done", 0, 20, "Done · 0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := string(captionLabel(c.lane, c.count, c.width)); got != c.want {
				t.Errorf("captionLabel(%q, %d, %d) = %q, want %q", c.lane, c.count, c.width, got, c.want)
			}
		})
	}
}

func drawCaptionRow(t *testing.T, gcr *GradientCaptionRow, width int) string {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(width, 1)
	gcr.SetRect(0, 0, width, 1)
	gcr.Draw(screen)
	return screenRow(screen, 0, width)
}

func screenRow(screen tcell.Screen, y, width int) string {
	var b strings.Builder
	for x := 0; x < width; x++ {
		str, _, _ := screen.Get(x, y)
		if str == "" {
			str = " "
		}
		b.WriteString(str)
	}
	return b.String()
}

func newTestCaptionRow(names []string) *GradientCaptionRow {
	bg := theme.NewColorRoleAdapter(theme.NewColor(tcell.ColorBlue))
	return NewGradientCaptionRow(names, nil, bg, theme.NewColor(tcell.ColorWhite))
}

func TestGradientCaptionRowDrawsLaneCounts(t *testing.T) {
	gcr := newTestCaptionRow([]string{"Ready", "Done"})
	gcr.SetLaneCounts([]int{12, 0})

	got := drawCaptionRow(t, gcr, 40)
	want := "     Ready · 12     " + "      Done · 0      "
	if got != want {
		t.Errorf("caption row = %q, want %q", got, want)
	}
}

func TestGradientCaptionRowWithoutCountsShowsNamesOnly(t *testing.T) {
	gcr := newTestCaptionRow([]string{"Wiki page"})

	got := strings.TrimSpace(drawCaptionRow(t, gcr, 30))
	if got != "Wiki page" {
		t.Errorf("caption = %q, want %q", got, "Wiki page")
	}
}
