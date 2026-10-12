package controller

import (
	"slices"
	"sort"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/boolean-maybe/tiki/model"
	"github.com/boolean-maybe/tiki/plugin"
	"github.com/boolean-maybe/tiki/store"
	tikipkg "github.com/boolean-maybe/tiki/tiki"
)

func deleteAction(t *testing.T, stmt string) plugin.PluginAction {
	t.Helper()
	parsed := mustParseStmt(t, stmt)
	a := plugin.PluginAction{
		Key: tcell.KeyDelete, KeyStr: "Delete", Label: "Delete", Kind: plugin.ActionKindRuki,
		Action: parsed, HasChoose: parsed.UsesChooseBuiltin(),
	}
	if a.HasChoose {
		a.ChooseFilter = parsed.ChooseFilter()
	}
	return a
}

func tikiIDs(tikis []*tikipkg.Tiki) []string {
	ids := make([]string, 0, len(tikis))
	for _, tk := range tikis {
		ids = append(ids, tk.ID())
	}
	sort.Strings(ids)
	return ids
}

func sameIDs(got []string, want ...string) bool {
	return slices.Equal(got, want)
}

func TestPluginController_PreviewDeleteListsWithoutDeleting(t *testing.T) {
	pc, tikiStore, _ := newCreateSelectBoard(t, deleteAction(t, `delete where id = id()`))
	selectExistingTiki(t, pc)

	got := tikiIDs(pc.PreviewDelete(pluginActionID("Delete")))

	if !sameIDs(got, "OLD001") {
		t.Errorf("preview = %v, want [OLD001]", got)
	}
	if tikiStore.GetTiki("OLD001") == nil {
		t.Error("preview deleted OLD001")
	}
}

func TestPluginController_PreviewDeleteFollowsWhereClause(t *testing.T) {
	pc, tikiStore, _ := newCreateSelectBoard(t, deleteAction(t, `delete where status = "ready"`))
	seedTiki(t, tikiStore, "OLD002", "Also ready", "ready", 0)
	seedTiki(t, tikiStore, "OLD003", "Finished", "done", 0)

	got := tikiIDs(pc.PreviewDelete(pluginActionID("Delete")))

	if !sameIDs(got, "OLD001", "OLD002") {
		t.Errorf("preview = %v, want [OLD001 OLD002]", got)
	}
}

func TestPluginController_PreviewDeleteIgnoresNonDeletes(t *testing.T) {
	update := plugin.PluginAction{
		Key: tcell.KeyRune, Rune: 'm', KeyStr: "m", Label: "Assign", Kind: plugin.ActionKindRuki,
		Action: mustParseStmt(t, `update where id = id() set assignee = "me"`),
	}
	pc, _, _ := newCreateSelectBoard(t, update)
	selectExistingTiki(t, pc)

	if got := pc.PreviewDelete(pluginActionID("m")); len(got) != 0 {
		t.Errorf("preview of an update = %v, want none", tikiIDs(got))
	}
}

func TestDetailController_PreviewDeleteListsShownTiki(t *testing.T) {
	dc, _, tikiStore := newDetailOverBoard(t, `delete where id = id()`)

	got := tikiIDs(dc.PreviewDelete(pluginActionID("Delete")))

	if !sameIDs(got, "TASK01") {
		t.Errorf("preview = %v, want [TASK01]", got)
	}
	if tikiStore.GetTiki("TASK01") == nil {
		t.Error("preview deleted TASK01")
	}
}

func TestWikiController_PreviewDeleteListsMatches(t *testing.T) {
	dc := newWikiTestController(t, []plugin.PluginAction{
		rukiGlobal(t, 'X', "Purge done", `delete where status = "done"`),
	})
	tikiStore := dc.executor.tikiStore
	seedTiki(t, tikiStore, "DONE01", "Finished", "done", 0)
	seedTiki(t, tikiStore, "OPEN01", "Open", "ready", 0)

	got := tikiIDs(dc.PreviewDelete(pluginActionID("X")))

	if !sameIDs(got, "DONE01") {
		t.Errorf("preview = %v, want [DONE01]", got)
	}
	if tikiStore.GetTiki("DONE01") == nil {
		t.Error("preview deleted DONE01")
	}
}

type fakeConfirmView struct{ shown []*tikipkg.Tiki }

func (f *fakeConfirmView) OnShow(tikis []*tikipkg.Tiki) { f.shown = tikis }

type fakeQuickSelectView struct{ shown bool }

func (f *fakeQuickSelectView) OnShow([]*tikipkg.Tiki)          { f.shown = true }
func (f *fakeQuickSelectView) GetFilterInput() tview.Primitive { return nil }

type confirmHarness struct {
	ir        *InputRouter
	pc        *PluginController
	tikiStore store.Store
	confirm   *model.ConfirmConfig
	view      *fakeConfirmView
}

func newConfirmHarness(t *testing.T, actions ...plugin.PluginAction) *confirmHarness {
	t.Helper()
	pc, tikiStore, nav := newCreateSelectBoard(t, actions...)
	nav.PushView(model.MakePluginViewID("TestPlugin"), nil)
	h := &confirmHarness{
		ir: &InputRouter{
			navController:     nav,
			pluginControllers: map[string]PluginControllerInterface{"TestPlugin": pc},
			globalActions:     DefaultGlobalActions(),
		},
		pc:        pc,
		tikiStore: tikiStore,
		confirm:   model.NewConfirmConfig(),
		view:      &fakeConfirmView{},
	}
	h.ir.SetConfirmConfig(h.confirm)
	h.ir.SetConfirmView(h.view)
	return h
}

func (h *confirmHarness) pressDelete() bool {
	return h.ir.handlePluginInput(tcell.NewEventKey(tcell.KeyDelete, 0, tcell.ModNone),
		model.MakePluginViewID("TestPlugin"))
}

func TestInputRouter_DeleteKeyAsksFirst(t *testing.T) {
	h := newConfirmHarness(t, deleteAction(t, `delete where id = id()`))
	selectExistingTiki(t, h.pc)

	if !h.pressDelete() {
		t.Fatal("Delete not handled")
	}

	if !h.confirm.IsVisible() {
		t.Error("confirmation not shown")
	}
	if got := tikiIDs(h.view.shown); !sameIDs(got, "OLD001") {
		t.Errorf("dialog shows %v, want [OLD001]", got)
	}
	if h.tikiStore.GetTiki("OLD001") == nil {
		t.Error("OLD001 deleted before confirming")
	}
}

func TestInputRouter_ConfirmedDeleteRuns(t *testing.T) {
	h := newConfirmHarness(t, deleteAction(t, `delete where id = id()`))
	selectExistingTiki(t, h.pc)
	h.pressDelete()

	h.confirm.Confirm()

	if h.tikiStore.GetTiki("OLD001") != nil {
		t.Error("OLD001 still in store after confirming")
	}
}

func TestInputRouter_CancelledDeleteKeepsTiki(t *testing.T) {
	h := newConfirmHarness(t, deleteAction(t, `delete where id = id()`))
	selectExistingTiki(t, h.pc)
	h.pressDelete()

	h.confirm.Cancel()

	if h.tikiStore.GetTiki("OLD001") == nil {
		t.Error("OLD001 deleted after cancelling")
	}
}

func TestInputRouter_PaletteDeleteAsksFirst(t *testing.T) {
	h := newConfirmHarness(t, deleteAction(t, `delete where id = id()`))
	selectExistingTiki(t, h.pc)

	h.ir.HandleAction(pluginActionID("Delete"), h.ir.navController.CurrentView())

	if !h.confirm.IsVisible() {
		t.Error("confirmation not shown")
	}
	if h.tikiStore.GetTiki("OLD001") == nil {
		t.Error("OLD001 deleted before confirming")
	}
}

func TestInputRouter_DeleteMatchingNothingDoesNotAsk(t *testing.T) {
	h := newConfirmHarness(t, deleteAction(t, `delete where status = "done"`))

	h.pressDelete()

	if h.confirm.IsVisible() {
		t.Error("confirmation shown for a delete that matches nothing")
	}
}

func TestInputRouter_NonDeleteRunsWithoutAsking(t *testing.T) {
	assign := plugin.PluginAction{
		Key: tcell.KeyRune, Rune: 'm', KeyStr: "m", Label: "Assign", Kind: plugin.ActionKindRuki,
		Action: mustParseStmt(t, `update where id = id() set assignee = "me"`),
	}
	h := newConfirmHarness(t, assign)
	selectExistingTiki(t, h.pc)

	h.ir.handlePluginInput(tcell.NewEventKey(tcell.KeyRune, 'm', tcell.ModNone),
		model.MakePluginViewID("TestPlugin"))

	if h.confirm.IsVisible() {
		t.Error("confirmation shown for an update")
	}
	if got, _, _ := h.tikiStore.GetTiki("OLD001").StringField("assignee"); got != "me" {
		t.Errorf("assignee = %q, want the update applied", got)
	}
}

// Picking the tiki in choose() is already a deliberate step.
func TestInputRouter_ChooseDeleteOpensPickerWithoutAsking(t *testing.T) {
	h := newConfirmHarness(t, deleteAction(t, `delete where id = choose(select where status = "ready")`))
	qsConfig := model.NewQuickSelectConfig()
	qsView := &fakeQuickSelectView{}
	h.ir.SetQuickSelectConfig(qsConfig)
	h.ir.SetQuickSelectView(qsView)

	h.pressDelete()

	if !qsConfig.IsVisible() || !qsView.shown {
		t.Error("choose() delete did not open the picker")
	}
	if h.confirm.IsVisible() {
		t.Error("confirmation shown for a choose() delete")
	}
}

// Live reload holds external changes while the dialog is open.
func TestInputRouter_ConfirmationIsModal(t *testing.T) {
	h := newConfirmHarness(t, deleteAction(t, `delete where id = id()`))
	selectExistingTiki(t, h.pc)

	h.pressDelete()

	if !h.ir.IsModalInteractionActive() {
		t.Error("open confirmation not reported as a modal interaction")
	}
}
