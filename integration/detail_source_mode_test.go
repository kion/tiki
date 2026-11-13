package integration

import (
	"testing"

	"github.com/boolean-maybe/tiki/controller"
	"github.com/boolean-maybe/tiki/model"
	"github.com/boolean-maybe/tiki/testutil"

	"github.com/gdamore/tcell/v2"
)

func createTikiWithBody(t *testing.T, ta *testutil.TestApp, id, title, body string) {
	t.Helper()
	if err := testutil.CreateTestTiki(ta.TikiDir, id, title, "ready", "story"); err != nil {
		t.Fatalf("create tiki %s: %v", id, err)
	}
	if err := ta.TikiStore.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	tk := ta.TikiStore.GetTiki(id).Clone()
	tk.SetBody(body)
	if err := ta.TikiStore.UpdateTiki(tk); err != nil {
		t.Fatalf("set body of %s: %v", id, err)
	}
}

func assertOnScreen(t *testing.T, ta *testutil.TestApp, want string) {
	t.Helper()
	if found, _, _ := ta.FindText(want); !found {
		ta.DumpScreen()
		t.Fatalf("%q not on screen", want)
	}
}

// The 80-column test screen has no room for view actions in the header,
// so the label is read from the registry the header and palette draw from.
func assertToggleLabel(t *testing.T, ta *testutil.TestApp, want string) {
	t.Helper()
	a := ta.RootLayout.GetContentView().GetActionRegistry().GetByID(controller.ActionToggleSource)
	if a == nil {
		t.Fatal("the current view has no source toggle")
	}
	if a.Label != want {
		t.Fatalf("toggle label = %q, want %q", a.Label, want)
	}
}

func openDetail(t *testing.T, ta *testutil.TestApp, id string) {
	t.Helper()
	if !ta.NavigateToTiki(id, 10) {
		t.Fatalf("could not select tiki %s on the board", id)
	}
	ta.SendKey(tcell.KeyEnter, 0, tcell.ModNone)
}

// The mode is app-wide: a detail view opened later starts in the mode the
// last one was left in, and the action is named after what `v` does next.
func TestDetailSourceMode_SticksFromTikiToTiki(t *testing.T) {
	ta := testutil.NewTestApp(t)
	defer ta.Cleanup()
	createTikiWithBody(t, ta, "000001", "First tiki", "Body *one* here.")
	createTikiWithBody(t, ta, "000002", "Second tiki", "Body *two* here.")

	ta.NavController.PushView(model.MakePluginViewID("Kanban"), nil)
	ta.Draw()

	openDetail(t, ta, "000001")
	assertOnScreen(t, ta, "Body one here.")
	assertToggleLabel(t, ta, "View source")

	ta.SendKey(tcell.KeyRune, 'v', tcell.ModNone)
	assertOnScreen(t, ta, "Body *one* here.")
	assertToggleLabel(t, ta, "View rendered")

	ta.SendKey(tcell.KeyEscape, 0, tcell.ModNone)
	openDetail(t, ta, "000002")
	assertOnScreen(t, ta, "Body *two* here.")
	assertToggleLabel(t, ta, "View rendered")

	ta.SendKey(tcell.KeyRune, 'v', tcell.ModNone)
	assertOnScreen(t, ta, "Body two here.")
	assertToggleLabel(t, ta, "View source")
}
