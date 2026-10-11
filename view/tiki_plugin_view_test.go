package view

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/boolean-maybe/tiki/controller"
	"github.com/boolean-maybe/tiki/model"
	"github.com/boolean-maybe/tiki/plugin"
	"github.com/boolean-maybe/tiki/store"
	tikipkg "github.com/boolean-maybe/tiki/tiki"
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
	}, nil, controller.PluginViewActions(), true)

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

	pv := NewPluginView(tikiStore, pluginConfig, pluginDef, provider, nil, controller.PluginViewActions(), true)

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
			}, nil, controller.PluginViewActions(), true)

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
			}, nil, controller.PluginViewActions(), true)

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
	}, nil, controller.PluginViewActions(), true)

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

// laneBoard is a mutable per-lane tiki list standing in for the
// controller's lane filters.
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
	return NewPluginView(store.NewInMemoryStore(), pc, pluginDef, board.get, nil, controller.PluginViewActions(), true)
}

func newSelectionConfig(lanes int) *model.PluginConfig {
	pc := model.NewPluginConfig("Board")
	cols := make([]int, lanes)
	for i := range cols {
		cols[i] = 1
	}
	pc.SetLaneLayout(cols, nil)
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

	pc.SetSelectedIndexForLane(0, 2) // the user moves down
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

	// while another view is shown, a controller selects index 0 and the
	// store changes; coming back must honour the controller's choice
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

	// an external insert lands while the user is in a detail view; the
	// board view is rebuilt on the way back
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
