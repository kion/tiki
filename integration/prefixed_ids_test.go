package integration

import (
	"testing"

	"github.com/boolean-maybe/tiki/controller"
	"github.com/boolean-maybe/tiki/model"
	"github.com/boolean-maybe/tiki/testutil"
	"github.com/boolean-maybe/tiki/workflow"

	"github.com/gdamore/tcell/v2"
)

func prefixedApp(t *testing.T) *testutil.TestApp {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ta := testutil.NewTestApp(t)
	workflow.SetProjectPrefix("TIK")
	t.Cleanup(func() { workflow.SetProjectPrefix("") })
	return ta
}

func TestPrefixedDraft_SaveAndCloseSelectsFinalID(t *testing.T) {
	ta := prefixedApp(t)
	defer ta.Cleanup()
	ta.NavController.PushView(model.MakePluginViewID("Kanban"), nil)
	ta.Draw()
	ta.SendKey(tcell.KeyRune, 'n', tcell.ModNone)
	ta.SendText("Fresh From Form")
	ta.SendKey(tcell.KeyEnter, 0, tcell.ModNone)
	ta.Draw()

	created := findTikiByTitle(ta.TikiStore.GetAllTikis(), "Fresh From Form")
	if created == nil || created.ID() != "TIK-1" {
		t.Fatalf("saved tiki = %v, want TIK-1", created)
	}
	sv, ok := ta.NavController.GetActiveView().(controller.SelectableView)
	if !ok {
		t.Fatal("the board view is not selectable")
	}
	if got := sv.GetSelectedID(); got != "TIK-1" {
		t.Errorf("board selected %q, want TIK-1", got)
	}
}

func TestPrefixedDraft_SaveAndStayKeepsActionsWorking(t *testing.T) {
	ta := prefixedApp(t)
	defer ta.Cleanup()
	ta.NavController.PushView(model.MakePluginViewID("Kanban"), nil)
	ta.Draw()
	ta.SendKey(tcell.KeyRune, 'n', tcell.ModNone)
	ta.SendText("Stay Open")
	ta.SendKey(tcell.KeyCtrlS, 0, tcell.ModCtrl)
	ta.Draw()

	created := findTikiByTitle(ta.TikiStore.GetAllTikis(), "Stay Open")
	if created == nil {
		t.Fatal("draft not saved")
	}
	if sv, ok := ta.NavController.GetActiveView().(controller.SelectableView); ok {
		if got := sv.GetSelectedID(); got != created.ID() {
			t.Errorf("detail view still holds %q after save, want %q", got, created.ID())
		}
	}
	// R = Make recurring: update where id = id() set recurrence=...
	ta.SendKey(tcell.KeyRune, 'R', tcell.ModNone)
	ta.Draw()
	if v, ok := ta.TikiStore.GetTiki(created.ID()).Get("recurrence"); !ok || v == nil || v == "" {
		msg, _, _ := ta.GetStatuslineConfig().GetMessage()
		t.Errorf("an id()-based action did nothing on the just-saved tiki (recurrence=%v, statusline=%q)", v, msg)
	}
}

func TestPrefixedLaneMove_ImmutableFieldShowsError(t *testing.T) {
	ta := prefixedApp(t)
	defer ta.Cleanup()
	defs := workflow.WorkflowFields()
	for i := range defs {
		if defs[i].Name == "status" {
			defs[i].Immutable = true
		}
	}
	if err := workflow.RegisterWorkflowFields(defs); err != nil {
		t.Fatal(err)
	}
	tk, err := ta.TikiStore.NewTikiTemplate()
	if err != nil {
		t.Fatal(err)
	}
	tk.SetTitle("Stuck")
	tk.Set("status", "inbox")
	if err := ta.TikiStore.CreateTiki(tk); err != nil {
		t.Fatal(err)
	}
	ta.NavController.PushView(model.MakePluginViewID("Kanban"), nil)
	ta.Draw()
	ta.SelectCursorTiki()
	ta.SendKey(tcell.KeyRight, 0, tcell.ModShift)
	ta.Draw()

	msg, level, _ := ta.GetStatuslineConfig().GetMessage()
	st, _, _ := ta.TikiStore.GetTiki(tk.ID()).StringField("status")
	if st != "inbox" || msg != "status is immutable" || level != model.MessageLevelError {
		t.Errorf("status=%q statusline=%q level=%v; want inbox, \"status is immutable\", error", st, msg, level)
	}
}
