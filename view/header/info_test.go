package header

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

func TestFitTitle_Fits(t *testing.T) {
	if got := fitTitle("Tiki fork", "Kanban", infoTextWidth); got != "Tiki fork" {
		t.Errorf("fitTitle = %q", got)
	}
}

func TestFitTitle_TruncatesProject(t *testing.T) {
	got := fitTitle("A very long project name that keeps going", "Kanban", infoTextWidth)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("fitTitle = %q, want a truncated name ending in …", got)
	}
	if w := runewidth.StringWidth(got + " • Kanban"); w != infoTextWidth {
		t.Errorf("title width = %d, want %d", w, infoTextWidth)
	}
}

func TestFitTitle_DropsProjectWhenViewFills(t *testing.T) {
	if got := fitTitle("Acme", strings.Repeat("v", infoTextWidth-3), infoTextWidth); got != "" {
		t.Errorf("fitTitle = %q, want the project dropped", got)
	}
}

func TestInfoWidget_ShowsProjectAndView(t *testing.T) {
	iw := NewInfoWidget()
	iw.SetProject("Acme [x]", "#ff8800")
	iw.SetViewInfo("Kanban", "Tasks")

	plain := iw.GetText(true)
	if !strings.HasPrefix(plain, "Acme [x] • Kanban\n") {
		t.Errorf("first line of %q, want 'Acme [x] • Kanban'", plain)
	}
	if !strings.Contains(plain, "\n"+strings.Repeat("─", infoTextWidth)+"\n") {
		t.Errorf("separator should be %d wide: %q", infoTextWidth, plain)
	}
	if !strings.Contains(iw.GetText(false), "[#ff8800") {
		t.Errorf("project name should use the project colour: %q", iw.GetText(false))
	}
}
