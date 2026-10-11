package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/boolean-maybe/tiki/model"
	"github.com/boolean-maybe/tiki/testutil"
)

const liveReloadWait = 10 * time.Second

func newLiveBoard(t *testing.T) *testutil.TestApp {
	t.Helper()
	ta := testutil.NewTestApp(t)
	t.Cleanup(ta.Cleanup)
	if err := ta.LoadPlugins(); err != nil {
		t.Fatalf("failed to load plugins: %v", err)
	}
	if err := testutil.CreateTestTiki(ta.TikiDir, "000001", "First Tiki", "ready", "story"); err != nil {
		t.Fatalf("failed to create test tiki: %v", err)
	}
	if err := ta.TikiStore.Reload(); err != nil {
		t.Fatalf("failed to reload tikis: %v", err)
	}
	ta.SetupLiveReload()
	ta.NavController.PushView(model.MakePluginViewID("Kanban"), nil)
	ta.Draw()
	return ta
}

func liveIndicatorShown(ta *testutil.TestApp) bool {
	for _, stat := range ta.GetStatuslineConfig().GetLeftStats() {
		if strings.Contains(stat.Value, "live") {
			return true
		}
	}
	return false
}

func TestLiveReload_ExternalCreateAppearsWithoutKeypress(t *testing.T) {
	ta := newLiveBoard(t)
	ta.SendKey(tcell.KeyCtrlL, 0, tcell.ModCtrl)

	if err := testutil.CreateTestTiki(ta.TikiDir, "000002", "Created Elsewhere", "ready", "story"); err != nil {
		t.Fatalf("failed to create external tiki: %v", err)
	}

	if !ta.PumpLiveReload(liveReloadWait, func() bool {
		found, _, _ := ta.FindText("000002")
		return found
	}) {
		ta.DumpScreen()
		t.Fatal("externally created tiki never appeared on the board")
	}
}

func TestLiveReload_CtrlLTogglesIndicator(t *testing.T) {
	ta := newLiveBoard(t)

	ta.SendKey(tcell.KeyCtrlL, 0, tcell.ModCtrl)
	if !liveIndicatorShown(ta) {
		t.Fatal("Ctrl-L did not turn live reload on")
	}

	ta.SendKey(tcell.KeyCtrlL, 0, tcell.ModCtrl)
	if liveIndicatorShown(ta) {
		t.Fatal("second Ctrl-L did not turn live reload off")
	}
}

func TestLiveReload_HeldWhilePaletteOpen(t *testing.T) {
	ta := newLiveBoard(t)
	ta.SendKey(tcell.KeyCtrlL, 0, tcell.ModCtrl)

	ta.SendKey(tcell.KeyCtrlA, 0, tcell.ModCtrl)
	if !ta.GetPaletteConfig().IsVisible() {
		t.Fatal("palette did not open")
	}
	if err := testutil.CreateTestTiki(ta.TikiDir, "000002", "Created Elsewhere", "ready", "story"); err != nil {
		t.Fatalf("failed to create external tiki: %v", err)
	}

	// so the absence check below isn't vacuous
	start := ta.LiveReloadUpdatesRun()
	if !ta.PumpLiveReload(liveReloadWait, func() bool { return ta.LiveReloadUpdatesRun() > start }) {
		t.Fatal("external change never reached the UI goroutine")
	}
	if ta.TikiStore.GetTiki("000002") != nil {
		t.Fatal("change applied while the palette was open")
	}

	ta.SendKey(tcell.KeyEscape, 0, tcell.ModNone)
	if !ta.PumpLiveReload(liveReloadWait, func() bool { return ta.TikiStore.GetTiki("000002") != nil }) {
		t.Fatal("held change not applied after the palette closed")
	}
}

func TestLiveReload_ExternalChangeWhileEditingWarns(t *testing.T) {
	ta := newLiveBoard(t)
	ta.SendKey(tcell.KeyEnter, 0, tcell.ModNone)  // open detail
	ta.SendKey(tcell.KeyRune, 'e', tcell.ModNone) // enter edit mode
	ta.SendKey(tcell.KeyCtrlL, 0, tcell.ModCtrl)

	if liveIndicatorShown(ta) {
		t.Fatal("Ctrl-L toggled live reload while a text field had focus")
	}
	if err := ta.LiveReload().SetEnabled(true); err != nil {
		t.Fatalf("enable live reload: %v", err)
	}

	if err := testutil.CreateTestTiki(ta.TikiDir, "000001", "Rewritten Elsewhere", "ready", "story"); err != nil {
		t.Fatalf("failed to rewrite tiki: %v", err)
	}

	if !ta.PumpLiveReload(liveReloadWait, func() bool {
		msg, _, _ := ta.GetStatuslineConfig().GetMessage()
		return strings.Contains(msg, "changed on disk")
	}) {
		msg, _, _ := ta.GetStatuslineConfig().GetMessage()
		t.Fatalf("no on-disk change warning while editing; statusline message %q", msg)
	}
}
