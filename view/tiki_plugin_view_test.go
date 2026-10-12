package view

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/boolean-maybe/tiki/controller"
	"github.com/boolean-maybe/tiki/model"
	"github.com/boolean-maybe/tiki/plugin"
	"github.com/boolean-maybe/tiki/store"
	"github.com/boolean-maybe/tiki/theme"
	tikipkg "github.com/boolean-maybe/tiki/tiki"

	"github.com/gdamore/tcell/v2"
)

func TestPluginViewRefreshResetsNonSelectedLaneScrollOffset(t *testing.T) {
	tikiStore := store.NewInMemoryStore()
	pluginConfig := model.NewPluginConfig("TestPlugin")
	pluginConfig.SetLaneLayout([]int{1, 1}, nil)

	pluginDef := &plugin.WorkflowPlugin{
		BasePlugin: plugin.BasePlugin{
			Name: "TestPlugin",
		},
		Lanes: []plugin.TikiLane{
			{Name: "Lane0", Columns: 1},
			{Name: "Lane1", Columns: 1},
		},
		Layout: testPluginLayout(t),
	}

	tikis := make([]*tikipkg.Tiki, 10)
	for i := range tikis {
		tk := tikipkg.New()
		tk.SetID(fmt.Sprintf("T-%d", i))
		tk.SetTitle(fmt.Sprintf("Tiki %d", i))
		tikis[i] = tk
	}

	pv := NewPluginView(tikiStore, pluginConfig, pluginDef, func(lane int) []*tikipkg.Tiki {
		return tikis
	}, nil, controller.PluginViewActions(), true, nil)

	itemHeight := 5 // 3-row layout + gridbox.TikiBoxOverhead (2)
	for _, lb := range pv.laneBoxes {
		lb.SetRect(0, 0, 80, itemHeight*5)
	}

	// select last tiki in lane 0 to force scroll offset
	pluginConfig.SetSelectedLane(0)
	pluginConfig.SetSelectedIndexForLane(0, len(tikis)-1)
	pv.refresh()

	if pv.laneBoxes[0].scrollOffset == 0 {
		t.Fatalf("expected lane 0 scroll offset > 0 after selecting last item")
	}

	// non-selected lane 1 must have scroll offset 0
	if pv.laneBoxes[1].scrollOffset != 0 {
		t.Fatalf("expected non-selected lane 1 scroll offset 0, got %d", pv.laneBoxes[1].scrollOffset)
	}

	// switch selection to lane 1, scroll it, then verify lane 0 resets
	pluginConfig.SetSelectedLane(1)
	pluginConfig.SetSelectedIndexForLane(1, len(tikis)-1)
	pv.refresh()

	if pv.laneBoxes[1].scrollOffset == 0 {
		t.Fatalf("expected lane 1 scroll offset > 0 after selecting last item")
	}
	if pv.laneBoxes[0].scrollOffset != 0 {
		t.Fatalf("expected non-selected lane 0 scroll offset 0, got %d", pv.laneBoxes[0].scrollOffset)
	}
}

// TestPluginViewStatsTrackSearchNarrowing reproduces the statusline
// count-vs-visible mismatch: when a search narrows the visible cards, the
// statusline must recompute its count too. RootLayout only refreshes stats
// when the active view fires actionChangeHandler, so this test models that
// contract — search must fire the handler, and GetStats() read at that moment
// must reflect the narrowed set, not the unfiltered total.
func TestPluginViewStatsTrackSearchNarrowing(t *testing.T) {
	tikiStore := store.NewInMemoryStore()
	pluginConfig := model.NewPluginConfig("TestPlugin")
	pluginConfig.SetLaneLayout([]int{1}, nil)

	pluginDef := &plugin.WorkflowPlugin{
		BasePlugin: plugin.BasePlugin{Name: "TestPlugin"},
		Lanes:      []plugin.TikiLane{{Name: "Lane", Columns: 1}},
		Layout:     testPluginLayout(t),
	}

	tikis := make([]*tikipkg.Tiki, 19)
	for i := range tikis {
		tk := tikipkg.New()
		tk.SetID(fmt.Sprintf("T-%d", i))
		tk.SetTitle(fmt.Sprintf("Tiki %d", i))
		tikis[i] = tk
	}

	// provider mirrors GetFilteredTikisForLane: narrows by active search results
	provider := func(_ int) []*tikipkg.Tiki {
		if results := pluginConfig.GetSearchResults(); results != nil {
			return results
		}
		return tikis
	}

	pv := NewPluginView(tikiStore, pluginConfig, pluginDef, provider, nil, controller.PluginViewActions(), true, nil)

	// mirror RootLayout: stats are recomputed only when the view signals a change
	var statsCount int
	readStats := func() {
		for _, s := range pv.GetStats() {
			if s.Name == "Total" {
				statsCount, _ = strconv.Atoi(s.Value)
			}
		}
	}
	pv.SetActionChangeHandler(readStats)

	pv.refresh() // initial render fires the handler
	if statsCount != 19 {
		t.Fatalf("initial total = %d, want 19", statsCount)
	}

	// apply a search that narrows to 3 — this fires notifyListeners → refresh → handler
	pluginConfig.SetSearchResults(tikis[:3], "query")
	pv.refresh()

	if statsCount != 3 {
		t.Fatalf("after search narrowing, total = %d, want 3 (statusline must track visible cards)", statsCount)
	}
}

func TestPluginViewCaptionAndHintsTrackLaneCounts(t *testing.T) {
	pluginConfig := model.NewPluginConfig("TestPlugin")
	pluginConfig.SetLaneLayout([]int{1, 4}, nil)
	pluginDef := &plugin.WorkflowPlugin{
		BasePlugin: plugin.BasePlugin{Name: "TestPlugin"},
		Lanes:      []plugin.TikiLane{{Name: "Lane0", Columns: 1}, {Name: "Lane1", Columns: 4}},
		Layout:     testPluginLayout(t),
	}

	tikis := make([]*tikipkg.Tiki, 10)
	for i := range tikis {
		tikis[i] = boardTiki(fmt.Sprintf("T%05d", i))
	}
	provider := func(lane int) []*tikipkg.Tiki {
		if lane == 1 {
			return nil
		}
		if results := pluginConfig.GetSearchResults(); results != nil {
			return results
		}
		return tikis
	}
	pv := NewPluginView(store.NewInMemoryStore(), pluginConfig, pluginDef, provider, nil, controller.PluginViewActions(), true, nil)

	pv.refresh()
	if got := fmt.Sprint(pv.titleBar.laneCounts); got != "[10 0]" {
		t.Errorf("caption counts = %s, want [10 0]", got)
	}
	if lb := pv.laneBoxes[0]; lb.entries != 10 || lb.perRow != 1 {
		t.Errorf("lane 0 hints count %d entries %d per row, want 10 per 1", lb.entries, lb.perRow)
	}
	if lb := pv.laneBoxes[1]; lb.perRow != 4 {
		t.Errorf("lane 1 hints count %d per row, want 4", lb.perRow)
	}

	pluginConfig.SetSearchResults(tikis[:3], "query")
	pv.refresh()
	if got := fmt.Sprint(pv.titleBar.laneCounts); got != "[3 0]" {
		t.Errorf("caption counts after search = %s, want [3 0]", got)
	}
}

func TestPluginViewGridLayout_RowCount(t *testing.T) {
	tests := []struct {
		name         string
		numTikis     int
		columns      int
		expectedRows int
	}{
		{"zero tikis", 0, 1, 0},
		{"6 tikis / 2 cols", 6, 2, 3},
		{"5 tikis / 3 cols", 5, 3, 2},
		{"1 tiki / 1 col", 1, 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tikiStore := store.NewInMemoryStore()
			pluginConfig := model.NewPluginConfig("TestPlugin")
			pluginConfig.SetLaneLayout([]int{tt.columns}, nil)

			pluginDef := &plugin.WorkflowPlugin{
				BasePlugin: plugin.BasePlugin{Name: "TestPlugin"},
				Lanes:      []plugin.TikiLane{{Name: "Lane", Columns: tt.columns}},
				Layout:     testPluginLayout(t),
			}

			tikis := make([]*tikipkg.Tiki, tt.numTikis)
			for i := range tikis {
				tk := tikipkg.New()
				tk.SetID(fmt.Sprintf("T-%d", i))
				tk.SetTitle(fmt.Sprintf("Tiki %d", i))
				tikis[i] = tk
			}

			pv := NewPluginView(tikiStore, pluginConfig, pluginDef, func(lane int) []*tikipkg.Tiki {
				return tikis
			}, nil, controller.PluginViewActions(), true, nil)

			pv.refresh()

			got := len(pv.laneBoxes[0].items)
			if got != tt.expectedRows {
				t.Errorf("rows = %d, want %d", got, tt.expectedRows)
			}
		})
	}
}

func TestPluginViewGridLayout_SelectedRow(t *testing.T) {
	tests := []struct {
		name                string
		numTikis            int
		columns             int
		selectedIndex       int
		expectedSelectedRow int
	}{
		{"index 0, 2 cols", 4, 2, 0, 0},
		{"index 2, 2 cols", 4, 2, 2, 1},
		{"index 4, 3 cols", 6, 3, 4, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tikiStore := store.NewInMemoryStore()
			pluginConfig := model.NewPluginConfig("TestPlugin")
			pluginConfig.SetLaneLayout([]int{tt.columns}, nil)

			pluginDef := &plugin.WorkflowPlugin{
				BasePlugin: plugin.BasePlugin{Name: "TestPlugin"},
				Lanes:      []plugin.TikiLane{{Name: "Lane", Columns: tt.columns}},
				Layout:     testPluginLayout(t),
			}

			tikis := make([]*tikipkg.Tiki, tt.numTikis)
			for i := range tikis {
				tk := tikipkg.New()
				tk.SetID(fmt.Sprintf("T-%d", i))
				tk.SetTitle(fmt.Sprintf("Tiki %d", i))
				tikis[i] = tk
			}

			pv := NewPluginView(tikiStore, pluginConfig, pluginDef, func(lane int) []*tikipkg.Tiki {
				return tikis
			}, nil, controller.PluginViewActions(), true, nil)

			pluginConfig.SetSelectedLane(0)
			pluginConfig.SetSelectedIndexForLane(0, tt.selectedIndex)
			pv.refresh()

			got := pv.laneBoxes[0].selectionIndex
			if got != tt.expectedSelectedRow {
				t.Errorf("selectionIndex = %d, want %d", got, tt.expectedSelectedRow)
			}
		})
	}
}

func TestPluginViewRefreshPreservesScrollOffset(t *testing.T) {
	tikiStore := store.NewInMemoryStore()
	pluginConfig := model.NewPluginConfig("TestPlugin")
	pluginConfig.SetLaneLayout([]int{1}, nil)

	pluginDef := &plugin.WorkflowPlugin{
		BasePlugin: plugin.BasePlugin{
			Name: "TestPlugin",
		},
		Lanes:  []plugin.TikiLane{{Name: "Lane", Columns: 1}},
		Layout: testPluginLayout(t),
	}

	tikis := make([]*tikipkg.Tiki, 10)
	for i := range tikis {
		tk := tikipkg.New()
		tk.SetID(fmt.Sprintf("T-%d", i))
		tk.SetTitle(fmt.Sprintf("Tiki %d", i))
		tikis[i] = tk
	}

	pv := NewPluginView(tikiStore, pluginConfig, pluginDef, func(lane int) []*tikipkg.Tiki {
		return tikis
	}, nil, controller.PluginViewActions(), true, nil)

	if len(pv.laneBoxes) != 1 {
		t.Fatalf("expected 1 lane box, got %d", len(pv.laneBoxes))
	}

	lane := pv.laneBoxes[0]
	itemHeight := 5 // 3-row layout + gridbox.TikiBoxOverhead (2)
	lane.SetRect(0, 0, 80, itemHeight*5)

	pluginConfig.SetSelectedIndexForLane(0, len(tikis)-1)
	pv.refresh()

	expectedScrollOffset := len(tikis) - 5
	if lane.scrollOffset != expectedScrollOffset {
		t.Fatalf("expected scrollOffset %d, got %d", expectedScrollOffset, lane.scrollOffset)
	}

	laneBefore := lane
	pluginConfig.SetSelectedIndexForLane(0, len(tikis)-2)
	pv.refresh()

	if pv.laneBoxes[0] != laneBefore {
		t.Fatalf("expected lane list to be reused across refresh")
	}

	if lane.scrollOffset != expectedScrollOffset {
		t.Fatalf("expected scrollOffset to remain %d, got %d", expectedScrollOffset, lane.scrollOffset)
	}
}

type laneBoard struct {
	lanes [][]*tikipkg.Tiki
}

func (b *laneBoard) get(lane int) []*tikipkg.Tiki { return b.lanes[lane] }

func boardTiki(id string) *tikipkg.Tiki {
	tk := tikipkg.New()
	tk.SetID(id)
	tk.SetTitle("tiki " + id)
	return tk
}

func newSelectionTestView(t *testing.T, board *laneBoard, pc *model.PluginConfig) *PluginView {
	t.Helper()
	lanes := make([]plugin.TikiLane, len(board.lanes))
	cols := make([]int, len(board.lanes))
	for i := range lanes {
		lanes[i] = plugin.TikiLane{Name: fmt.Sprintf("Lane%d", i), Columns: 1}
		cols[i] = 1
	}
	pluginDef := &plugin.WorkflowPlugin{
		BasePlugin: plugin.BasePlugin{Name: pc.GetPluginName()},
		Lanes:      lanes,
		Layout:     testPluginLayout(t),
	}
	return NewPluginView(store.NewInMemoryStore(), pc, pluginDef, board.get, nil, controller.PluginViewActions(), true, nil)
}

func newSelectionConfig(lanes int) *model.PluginConfig {
	pc := model.NewPluginConfig("Board")
	cols := make([]int, lanes)
	for i := range cols {
		cols[i] = 1
	}
	pc.SetLaneLayout(cols, nil)
	pc.ResumeSelection()
	return pc
}

func TestPluginViewKeepsSelectedTikiWhenOneIsInsertedAbove(t *testing.T) {
	board := &laneBoard{lanes: [][]*tikipkg.Tiki{{boardTiki("AAAAAA"), boardTiki("BBBBBB"), boardTiki("CCCCCC")}}}
	pc := newSelectionConfig(1)
	pv := newSelectionTestView(t, board, pc)
	pc.SetSelectedIndexForLane(0, 1)
	pv.refresh()
	if got := pv.GetSelectedID(); got != "BBBBBB" {
		t.Fatalf("precondition: selected %s", got)
	}

	board.lanes[0] = append([]*tikipkg.Tiki{boardTiki("NEWNEW")}, board.lanes[0]...)
	pv.refresh() // a store change

	if got := pv.GetSelectedID(); got != "BBBBBB" {
		t.Errorf("selected %s after an insert above, want BBBBBB", got)
	}
}

func TestPluginViewUserMoveIsNotUndone(t *testing.T) {
	board := &laneBoard{lanes: [][]*tikipkg.Tiki{{boardTiki("AAAAAA"), boardTiki("BBBBBB"), boardTiki("CCCCCC")}}}
	pc := newSelectionConfig(1)
	pv := newSelectionTestView(t, board, pc)
	pc.SetSelectedIndexForLane(0, 1)
	pv.refresh()

	pc.SetSelectedIndexForLane(0, 2)
	pv.refresh()

	if got := pv.GetSelectedID(); got != "CCCCCC" {
		t.Errorf("selected %s, want the user's choice CCCCCC", got)
	}
}

func TestPluginViewSelectionSetWhileHiddenSurvives(t *testing.T) {
	board := &laneBoard{lanes: [][]*tikipkg.Tiki{{boardTiki("AAAAAA"), boardTiki("BBBBBB"), boardTiki("CCCCCC")}}}
	pc := newSelectionConfig(1)
	pv := newSelectionTestView(t, board, pc)
	pc.SetSelectedIndexForLane(0, 1)
	pv.refresh()

	// set by a controller while the board was hidden
	pc.SetSelectedIndexForLane(0, 0)
	board.lanes[0] = append([]*tikipkg.Tiki{boardTiki("NEWNEW")}, board.lanes[0]...)
	back := newSelectionTestView(t, board, pc)

	if got := back.GetSelectedID(); got != "NEWNEW" {
		t.Errorf("selected %s, want NEWNEW at the controller-set index", got)
	}
}

func TestPluginViewRestoresSelectionAcrossViewRebuild(t *testing.T) {
	board := &laneBoard{lanes: [][]*tikipkg.Tiki{{boardTiki("AAAAAA"), boardTiki("BBBBBB"), boardTiki("CCCCCC")}}}
	pc := newSelectionConfig(1)
	pv := newSelectionTestView(t, board, pc)
	pc.SetSelectedIndexForLane(0, 1)
	pv.refresh()

	// a rebuilt view, as when returning from the detail view
	board.lanes[0] = append([]*tikipkg.Tiki{boardTiki("NEWNEW")}, board.lanes[0]...)
	back := newSelectionTestView(t, board, pc)

	if got := back.GetSelectedID(); got != "BBBBBB" {
		t.Errorf("selected %s after returning, want BBBBBB", got)
	}
}

func TestPluginViewDeletedSelectionClamps(t *testing.T) {
	board := &laneBoard{lanes: [][]*tikipkg.Tiki{{boardTiki("AAAAAA"), boardTiki("BBBBBB"), boardTiki("CCCCCC")}}}
	pc := newSelectionConfig(1)
	pv := newSelectionTestView(t, board, pc)
	pc.SetSelectedIndexForLane(0, 2)
	pv.refresh()

	board.lanes[0] = board.lanes[0][:2]
	pv.refresh()

	if got := pv.GetSelectedID(); got != "BBBBBB" {
		t.Errorf("selected %s after deleting the last tiki, want BBBBBB", got)
	}
}

func TestPluginViewSelectionStaysInLaneWhenTikiMovesAway(t *testing.T) {
	board := &laneBoard{lanes: [][]*tikipkg.Tiki{
		{boardTiki("AAAAAA"), boardTiki("BBBBBB"), boardTiki("CCCCCC")},
		{},
	}}
	pc := newSelectionConfig(2)
	pv := newSelectionTestView(t, board, pc)
	pc.SetSelectedIndexForLane(0, 1)
	pv.refresh()

	b := board.lanes[0][1]
	board.lanes[0] = []*tikipkg.Tiki{board.lanes[0][0], board.lanes[0][2]}
	board.lanes[1] = []*tikipkg.Tiki{b}
	pv.refresh()

	if lane := pc.GetSelectedLane(); lane != 0 {
		t.Errorf("selection followed the tiki to lane %d, want to stay in lane 0", lane)
	}
	if got := pv.GetSelectedID(); got != "CCCCCC" {
		t.Errorf("selected %s, want CCCCCC at the same index", got)
	}
}

func focusBorderCells(t *testing.T, pv *PluginView) int {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(80, 30)
	root := pv.GetPrimitive()
	root.SetRect(0, 0, 80, 30)
	root.Draw(screen)

	focus := theme.Roles().BorderFocus().TCell()
	count := 0
	for y := 0; y < 30; y++ {
		for x := 0; x < 80; x++ {
			_, style, _ := screen.Get(x, y)
			if fg, _, _ := style.Decompose(); fg == focus {
				count++
			}
		}
	}
	return count
}

func TestPluginViewHighlightsOnlyAnExplicitSelection(t *testing.T) {
	board := &laneBoard{lanes: [][]*tikipkg.Tiki{{boardTiki("AAAAAA"), boardTiki("BBBBBB")}}}
	pc := model.NewPluginConfig("Board")
	pc.SetLaneLayout([]int{1}, nil)
	pv := newSelectionTestView(t, board, pc)

	if got := pv.GetSelectedID(); got != "" {
		t.Errorf("selected %s on a new board, want nothing", got)
	}
	if n := focusBorderCells(t, pv); n != 0 {
		t.Errorf("%d highlighted cells on a new board, want 0", n)
	}

	pc.ResumeSelection()
	pv.refresh()

	if got := pv.GetSelectedID(); got != "AAAAAA" {
		t.Errorf("selected %s after resuming, want AAAAAA", got)
	}
	if n := focusBorderCells(t, pv); n == 0 {
		t.Error("no highlighted cells after resuming, want the selected card's border")
	}
}

func TestPluginViewHiddenCursorFollowsItsTiki(t *testing.T) {
	board := &laneBoard{lanes: [][]*tikipkg.Tiki{{boardTiki("AAAAAA"), boardTiki("BBBBBB"), boardTiki("CCCCCC")}}}
	pc := newSelectionConfig(1)
	pv := newSelectionTestView(t, board, pc)
	pc.SetSelectedIndexForLane(0, 1)
	pv.refresh()

	if !pv.ClearSelection() {
		t.Fatal("ClearSelection() = false with BBBBBB selected")
	}
	pv.refresh()
	board.lanes[0] = append([]*tikipkg.Tiki{boardTiki("NEWNEW")}, board.lanes[0]...)
	pv.refresh()
	pc.ResumeSelection()
	pv.refresh()

	if got := pv.GetSelectedID(); got != "BBBBBB" {
		t.Errorf("selected %s after resuming, want BBBBBB", got)
	}
}

func TestPluginViewClearSelectionWithNothingSelected(t *testing.T) {
	board := &laneBoard{lanes: [][]*tikipkg.Tiki{{boardTiki("AAAAAA")}}}
	pc := model.NewPluginConfig("Board")
	pc.SetLaneLayout([]int{1}, nil)
	pv := newSelectionTestView(t, board, pc)

	if pv.ClearSelection() {
		t.Error("ClearSelection() = true with nothing selected, want false so Esc can go back")
	}
}

func TestPluginViewSetSelectedIDSelects(t *testing.T) {
	board := &laneBoard{lanes: [][]*tikipkg.Tiki{{boardTiki("AAAAAA")}, {boardTiki("BBBBBB")}}}
	pc := model.NewPluginConfig("Board")
	pc.SetLaneLayout([]int{1, 1}, nil)
	pv := newSelectionTestView(t, board, pc)

	pv.SetSelectedID("BBBBBB")

	if got := pv.GetSelectedID(); got != "BBBBBB" {
		t.Errorf("selected %q, want BBBBBB", got)
	}
}
