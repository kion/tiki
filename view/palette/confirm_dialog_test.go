package palette

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/boolean-maybe/tiki/model"
	tikipkg "github.com/boolean-maybe/tiki/tiki"
)

func confirmTiki(id, title string) *tikipkg.Tiki {
	tk := tikipkg.New()
	tk.SetID(id)
	tk.SetTitle(title)
	return tk
}

func shownDialog(tikis ...*tikipkg.Tiki) (*ConfirmDialog, *model.ConfirmConfig, *bool) {
	cfg := model.NewConfirmConfig()
	ran := false
	cfg.SetOnConfirm(func() { ran = true })
	d := NewConfirmDialog(cfg)
	d.OnShow(tikis)
	cfg.SetVisible(true)
	return d, cfg, &ran
}

func pressKey(d *ConfirmDialog, ev *tcell.EventKey) {
	d.GetPrimitive().InputHandler()(ev, func(tview.Primitive) {})
}

// renderDialog draws the dialog at the size it asks for and returns its rows.
func renderDialog(t *testing.T, d *ConfirmDialog) []string {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("init screen: %v", err)
	}
	defer screen.Fini()
	w, h := d.Size()
	screen.SetSize(w, h)
	p := d.GetPrimitive()
	p.SetRect(0, 0, w, h)
	p.Draw(screen)
	screen.Show()

	cells, cw, ch := screen.GetContents()
	rows := make([]string, ch)
	for y := 0; y < ch; y++ {
		var b strings.Builder
		for x := 0; x < cw; x++ {
			r := cells[y*cw+x].Runes
			if len(r) == 0 {
				b.WriteRune(' ')
				continue
			}
			b.WriteRune(r[0])
		}
		rows[y] = b.String()
	}
	return rows
}

func TestConfirmDialog_EnterConfirms(t *testing.T) {
	d, cfg, ran := shownDialog(confirmTiki("ABC123", "Doomed"))

	pressKey(d, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))

	if !*ran {
		t.Error("Enter did not confirm")
	}
	if cfg.IsVisible() {
		t.Error("dialog still visible after Enter")
	}
}

func TestConfirmDialog_EscCancels(t *testing.T) {
	d, cfg, ran := shownDialog(confirmTiki("ABC123", "Doomed"))

	pressKey(d, tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))

	if *ran {
		t.Error("Esc confirmed the delete")
	}
	if cfg.IsVisible() {
		t.Error("dialog still visible after Esc")
	}
}

// A stray key leaves the question open, so it's never unclear whether the tiki was deleted.
func TestConfirmDialog_OtherKeysDoNothing(t *testing.T) {
	keys := []*tcell.EventKey{
		tcell.NewEventKey(tcell.KeyRune, 'y', tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, 'n', tcell.ModNone),
		tcell.NewEventKey(tcell.KeyDelete, 0, tcell.ModNone),
		tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone),
	}
	for _, ev := range keys {
		d, cfg, ran := shownDialog(confirmTiki("ABC123", "Doomed"))

		pressKey(d, ev)

		if *ran {
			t.Errorf("%s confirmed the delete", ev.Name())
		}
		if !cfg.IsVisible() {
			t.Errorf("%s closed the dialog", ev.Name())
		}
	}
}

func TestConfirmDialog_ShowsTikiBeingDeleted(t *testing.T) {
	d, _, _ := shownDialog(confirmTiki("ABC123", "Fix login redirect"))

	screen := strings.Join(renderDialog(t, d), "\n")

	for _, want := range []string{"Delete tiki?", "ABC123", "Fix login redirect", "Enter", "Esc"} {
		if !strings.Contains(screen, want) {
			t.Errorf("dialog missing %q:\n%s", want, screen)
		}
	}
}

func TestConfirmDialog_CountsSeveralTikis(t *testing.T) {
	d, _, _ := shownDialog(confirmTiki("AAA111", "One"), confirmTiki("BBB222", "Two"))

	screen := strings.Join(renderDialog(t, d), "\n")

	for _, want := range []string{"Delete 2 tikis?", "AAA111", "BBB222"} {
		if !strings.Contains(screen, want) {
			t.Errorf("dialog missing %q:\n%s", want, screen)
		}
	}
}

func TestConfirmDialog_CapsLongLists(t *testing.T) {
	var tikis []*tikipkg.Tiki
	for i := 1; i <= 7; i++ {
		tikis = append(tikis, confirmTiki(fmt.Sprintf("TIKI0%d", i), fmt.Sprintf("Task %d", i)))
	}
	d, _, _ := shownDialog(tikis...)

	screen := strings.Join(renderDialog(t, d), "\n")

	for _, want := range []string{"TIKI01", "TIKI05", "2 more"} {
		if !strings.Contains(screen, want) {
			t.Errorf("dialog missing %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, "TIKI06") {
		t.Errorf("dialog lists TIKI06, want it folded into the count:\n%s", screen)
	}
}

// A title is user text, so tview color tags in it must print literally.
func TestConfirmDialog_TitleIsNotMarkup(t *testing.T) {
	d, _, _ := shownDialog(confirmTiki("ABC123", "Fix [red] tags"))

	screen := strings.Join(renderDialog(t, d), "\n")

	if !strings.Contains(screen, "Fix [red] tags") {
		t.Errorf("title not shown literally:\n%s", screen)
	}
}

// tview hands keys to the root Pages, which only forwards them to a page that reports focus.
func TestConfirmOverlay_RoutesKeysToDialog(t *testing.T) {
	d, _, ran := shownDialog(confirmTiki("ABC123", "Doomed"))
	pages := tview.NewPages()
	pages.AddPage("base", tview.NewBox(), true, true)
	pages.AddPage("confirm", NewConfirmOverlay(d), true, true)
	app := tview.NewApplication()
	app.SetRoot(pages, true)
	app.SetFocus(d.GetPrimitive())

	if !pages.HasFocus() {
		t.Fatal("pages do not see the dialog's focus, so tview would drop its keys")
	}
	pages.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone),
		func(p tview.Primitive) { app.SetFocus(p) })

	if !*ran {
		t.Error("Enter did not reach the dialog through the pages")
	}
}
