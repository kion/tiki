package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/boolean-maybe/tiki/controller"
	"github.com/boolean-maybe/tiki/internal/livestatus"
	"github.com/boolean-maybe/tiki/model"
	"github.com/boolean-maybe/tiki/testutil"
)

func writeLiveStatus(t *testing.T, ta *testutil.TestApp, message string) {
	t.Helper()
	st := livestatus.Status{Agent: "✻", Message: message, UpdatedAt: time.Now()}
	if err := livestatus.Write(ta.TikiDir, "000001", st); err != nil {
		t.Fatalf("write live status: %v", err)
	}
}

// FindText reports byte offsets, not screen columns.
func columnOf(ta *testutil.TestApp, y int, r rune) int {
	contents, width, _ := ta.Screen.GetContents()
	for x := 0; x < width; x++ {
		if c := contents[y*width+x]; len(c.Runes) > 0 && c.Runes[0] == r {
			return x
		}
	}
	return -1
}

func shows(ta *testutil.TestApp, text string) func() bool {
	return func() bool {
		found, _, _ := ta.FindText(text)
		return found
	}
}

func TestLiveStatus_AppearsUpdatesAndClears(t *testing.T) {
	ta := newLiveBoard(t)
	ta.SendKey(tcell.KeyCtrlL, 0, tcell.ModCtrl)

	writeLiveStatus(t, ta, "Testing")
	if !ta.PumpLiveReload(liveReloadWait, shows(ta, "Testing")) {
		ta.DumpScreen()
		t.Fatal("live status never appeared on the card")
	}

	writeLiveStatus(t, ta, "Fixing")
	if !ta.PumpLiveReload(liveReloadWait, shows(ta, "Fixing")) {
		ta.DumpScreen()
		t.Fatal("live status update never appeared")
	}

	if err := livestatus.Remove(ta.TikiDir, "000001"); err != nil {
		t.Fatal(err)
	}
	if !ta.PumpLiveReload(liveReloadWait, func() bool { return !shows(ta, "Fixing")() }) {
		ta.DumpScreen()
		t.Fatal("cleared live status still shown")
	}
}

func TestLiveStatus_HiddenWhileLiveReloadIsOff(t *testing.T) {
	ta := newLiveBoard(t)
	writeLiveStatus(t, ta, "Testing")
	ta.Draw()
	if shows(ta, "Testing")() {
		t.Fatal("live status shown while live reload is off")
	}

	ta.SendKey(tcell.KeyCtrlL, 0, tcell.ModCtrl)
	ta.Draw()
	if !shows(ta, "Testing")() {
		ta.DumpScreen()
		t.Fatal("live status not shown once live reload turned on")
	}

	ta.SendKey(tcell.KeyCtrlL, 0, tcell.ModCtrl)
	ta.Draw()
	if shows(ta, "Testing")() {
		t.Fatal("live status still shown after live reload turned off")
	}
}

func TestLiveStatus_NotHeldWhilePaletteOpen(t *testing.T) {
	ta := newLiveBoard(t)
	ta.SendKey(tcell.KeyCtrlL, 0, tcell.ModCtrl)
	ta.SendKey(tcell.KeyCtrlA, 0, tcell.ModCtrl)
	if !ta.GetPaletteConfig().IsVisible() {
		t.Fatal("palette did not open")
	}

	start := ta.LiveReloadUpdatesRun()
	writeLiveStatus(t, ta, "Testing")
	if !ta.PumpLiveReload(liveReloadWait, func() bool { return ta.LiveReloadUpdatesRun() > start }) {
		t.Fatal("live status change never reached the UI goroutine")
	}

	// nothing is pumped after closing, so the status must have been applied while the palette was open
	ta.SendKey(tcell.KeyEscape, 0, tcell.ModNone)
	ta.Draw()
	if !shows(ta, "Testing")() {
		ta.DumpScreen()
		t.Fatal("live status was held while the palette was open")
	}
}

func TestLiveStatus_AnimatesWithoutInput(t *testing.T) {
	ta := newLiveBoard(t)
	ta.SendKey(tcell.KeyCtrlL, 0, tcell.ModCtrl)

	writeLiveStatus(t, ta, "Testing")
	// only redraws driven by the animator can step through the frames and come round again
	for _, frame := range []string{"✢", "✳", "✢"} {
		if !ta.PumpLiveReload(liveReloadWait, shows(ta, frame)) {
			ta.DumpScreen()
			t.Fatalf("animation never showed frame %s", frame)
		}
	}
}

func TestLiveStatus_StopsBeforeTheOverflowHint(t *testing.T) {
	ta := testutil.NewTestApp(t)
	t.Cleanup(ta.Cleanup)
	if err := ta.LoadPlugins(); err != nil {
		t.Fatalf("failed to load plugins: %v", err)
	}
	for i := 1; i <= 12; i++ {
		id := fmt.Sprintf("%06d", i)
		if err := testutil.CreateTestTiki(ta.TikiDir, id, "Tiki "+id, "ready", "story"); err != nil {
			t.Fatal(err)
		}
		if err := livestatus.Write(ta.TikiDir, id, livestatus.Status{Agent: "✻", Message: "Implementing", UpdatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	if err := ta.TikiStore.Reload(); err != nil {
		t.Fatal(err)
	}
	ta.SetupLiveReload()
	ta.NavController.PushView(model.MakePluginViewID("Kanban"), nil)
	ta.SendKey(tcell.KeyCtrlL, 0, tcell.ModCtrl)
	ta.Draw()

	_, _, y := ta.FindText("▼")
	x := columnOf(ta, y, '▼')
	if x < 0 {
		ta.DumpScreen()
		t.Fatal("no overflow hint on an overflowing lane")
	}
	left := ta.GetTextAt(0, y, x)
	if !regexp.MustCompile(`^└─ .*\S ─+$`).MatchString(left) {
		ta.DumpScreen()
		t.Fatalf("bottom border before the hint = %q, want the status, a space, then a dash", left)
	}
}

func TestLiveStatus_ClearFromThePalette(t *testing.T) {
	ta := newLiveBoard(t)
	ta.SendKey(tcell.KeyCtrlL, 0, tcell.ModCtrl)
	writeLiveStatus(t, ta, "Testing")
	if !ta.PumpLiveReload(liveReloadWait, shows(ta, "Testing")) {
		ta.DumpScreen()
		t.Fatal("live status never appeared on the card")
	}

	ta.SelectCursorTiki()
	if !ta.InputRouter.HandleAction(controller.ActionClearLiveStatus, ta.NavController.CurrentView()) {
		t.Fatal("Clear Live Status was not handled")
	}
	// no pump: the card must update without waiting for the watcher
	ta.Draw()
	if shows(ta, "Testing")() {
		t.Fatal("status still shown after Clear Live Status")
	}
	if _, ok, _ := livestatus.Read(ta.TikiDir, "000001"); ok {
		t.Fatal("status file still on disk after Clear Live Status")
	}
}

func TestLiveStatus_ShownInTheDetailView(t *testing.T) {
	ta := newLiveBoard(t)
	ta.SendKey(tcell.KeyCtrlL, 0, tcell.ModCtrl)
	params := model.EncodePluginViewParams(model.PluginViewParams{TikiID: "000001"})
	ta.NavController.PushView(model.MakePluginViewID("Detail"), params)
	ta.Draw()

	writeLiveStatus(t, ta, "Testing")
	if !ta.PumpLiveReload(liveReloadWait, shows(ta, "Testing")) {
		ta.DumpScreen()
		t.Fatal("live status never appeared in the detail view")
	}
	if got, want := ta.NavController.CurrentViewID(), model.MakePluginViewID("Detail"); got != want {
		t.Fatalf("current view = %q, want %q", got, want)
	}
}

func TestLiveStatus_ClearFromTheDetailViewPalette(t *testing.T) {
	ta := newLiveBoard(t)
	ta.SendKey(tcell.KeyCtrlL, 0, tcell.ModCtrl)
	params := model.EncodePluginViewParams(model.PluginViewParams{TikiID: "000001"})
	ta.NavController.PushView(model.MakePluginViewID("Detail"), params)
	ta.Draw()
	writeLiveStatus(t, ta, "Testing")
	if !ta.PumpLiveReload(liveReloadWait, shows(ta, "Testing")) {
		ta.DumpScreen()
		t.Fatal("live status never appeared in the detail view")
	}

	ta.SendKey(tcell.KeyCtrlA, 0, tcell.ModCtrl)
	ta.SendText("Clear Live Status")
	ta.SendKey(tcell.KeyEnter, 0, tcell.ModNone)
	// no pump: the view must update without waiting for the watcher
	ta.Draw()
	if shows(ta, "Testing")() {
		ta.DumpScreen()
		t.Fatal("status still shown after Clear Live Status")
	}
	if _, ok, _ := livestatus.Read(ta.TikiDir, "000001"); ok {
		t.Fatal("status file still on disk after Clear Live Status")
	}
	if msg, _, _ := ta.GetStatuslineConfig().GetMessage(); msg != "cleared the live status of 000001" {
		t.Fatalf("statusline message = %q", msg)
	}
}

// 000002 changed more recently than 000001, so it leads Recent until an agent works on 000001.
func newRecentBoard(t *testing.T) *testutil.TestApp {
	t.Helper()
	ta := testutil.NewTestApp(t)
	t.Cleanup(ta.Cleanup)
	if err := ta.LoadPlugins(); err != nil {
		t.Fatalf("failed to load plugins: %v", err)
	}
	for i, age := range []time.Duration{2 * time.Hour, time.Hour} {
		id := fmt.Sprintf("%06d", i+1)
		if err := testutil.CreateTestTiki(ta.TikiDir, id, "Tiki "+id, "ready", "story"); err != nil {
			t.Fatal(err)
		}
		at := time.Now().Add(-age)
		if err := os.Chtimes(filepath.Join(ta.TikiDir, id+".md"), at, at); err != nil {
			t.Fatal(err)
		}
	}
	if err := ta.TikiStore.Reload(); err != nil {
		t.Fatal(err)
	}
	ta.SetupLiveReload()
	ta.NavController.PushView(model.MakePluginViewID("Recent"), nil)
	ta.SendKey(tcell.KeyCtrlL, 0, tcell.ModCtrl)
	ta.Draw()
	return ta
}

func recentLeader(t *testing.T, ta *testutil.TestApp) string {
	t.Helper()
	pc, ok := ta.PluginControllers["Recent"].(*controller.PluginController)
	if !ok {
		t.Fatalf("Recent controller is %T", ta.PluginControllers["Recent"])
	}
	if tikis := pc.GetFilteredTikisForLane(0); len(tikis) > 0 {
		return tikis[0].ID()
	}
	return ""
}

func leadsOnScreen(ta *testutil.TestApp, id, other string) func() bool {
	return func() bool {
		found, x, y := ta.FindText(id)
		foundOther, ox, oy := ta.FindText(other)
		return found && foundOther && (y < oy || y == oy && x < ox)
	}
}

func TestLiveStatus_LeadsRecent(t *testing.T) {
	ta := newRecentBoard(t)
	if !leadsOnScreen(ta, "000002", "000001")() {
		ta.DumpScreen()
		t.Fatal("the more recently changed tiki doesn't lead Recent")
	}
	// the first key only resumes the selection on the first card
	ta.SendKey(tcell.KeyRight, 0, tcell.ModNone)
	ta.SendKey(tcell.KeyRight, 0, tcell.ModNone)
	sel, ok := ta.NavController.GetActiveView().(interface{ GetSelectedID() string })
	if !ok {
		t.Fatal("Recent view has no selection")
	}
	if got := sel.GetSelectedID(); got != "000001" {
		t.Fatalf("selected %q, want 000001", got)
	}

	writeLiveStatus(t, ta, "Testing")
	if !ta.PumpLiveReload(liveReloadWait, leadsOnScreen(ta, "000001", "000002")) {
		ta.DumpScreen()
		t.Fatal("the tiki with the newest live status never led Recent")
	}
	if got := sel.GetSelectedID(); got != "000001" {
		t.Fatalf("selection moved to %q when the lane reordered", got)
	}
}

func TestLiveStatus_RecentHeldWhilePaletteOpen(t *testing.T) {
	ta := newRecentBoard(t)
	ta.SendKey(tcell.KeyCtrlA, 0, tcell.ModCtrl)
	if !ta.GetPaletteConfig().IsVisible() {
		t.Fatal("palette did not open")
	}

	start := ta.LiveReloadUpdatesRun()
	writeLiveStatus(t, ta, "Testing")
	// the repaint, then a few busy rechecks
	if !ta.PumpLiveReload(liveReloadWait, func() bool { return ta.LiveReloadUpdatesRun() > start+3 }) {
		t.Fatal("live status change never reached the UI goroutine")
	}
	if got := recentLeader(t, ta); got != "000002" {
		t.Fatalf("Recent reordered to lead with %s while the palette was open", got)
	}

	ta.SendKey(tcell.KeyEscape, 0, tcell.ModNone)
	if !ta.PumpLiveReload(liveReloadWait, leadsOnScreen(ta, "000001", "000002")) {
		ta.DumpScreen()
		t.Fatal("Recent never reordered after the palette closed")
	}
}
