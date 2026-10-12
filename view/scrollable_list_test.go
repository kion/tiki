package view

import (
	"strings"
	"testing"

	"github.com/boolean-maybe/tiki/theme"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// mockPrimitive is a simple primitive for testing
type mockPrimitive struct {
	*tview.Box
}

func newMockPrimitive() *mockPrimitive {
	return &mockPrimitive{
		Box: tview.NewBox(),
	}
}

// Helper to create a scrollable list with N items for testing
func createTestList(itemCount int, itemHeight int) *ScrollableList {
	list := NewScrollableList().SetItemHeight(itemHeight)
	for i := 0; i < itemCount; i++ {
		list.AddItem(newMockPrimitive())
	}
	return list
}

// Helper to simulate setting a fixed height for the list
func setListHeight(list *ScrollableList, height int) {
	// Set the rect to simulate screen dimensions
	list.SetRect(0, 0, 100, height)
}

// TestScrollingDown tests scrolling down through the list
func TestScrollingDown(t *testing.T) {
	// 10 items, each 5 units tall, viewport shows 5 items (height=25)
	list := createTestList(10, 5)
	setListHeight(list, 25) // Can show 5 items (25/5=5)

	// Start at item 0
	list.SetSelection(0)
	if list.scrollOffset != 0 {
		t.Errorf("Initial scrollOffset should be 0, got %d", list.scrollOffset)
	}

	// Move down to item 1 - should not scroll yet
	list.SetSelection(1)
	if list.scrollOffset != 0 {
		t.Errorf("At item 1, scrollOffset should be 0, got %d", list.scrollOffset)
	}

	// Move down to item 4 (last visible in initial view) - should not scroll
	list.SetSelection(4)
	if list.scrollOffset != 0 {
		t.Errorf("At item 4, scrollOffset should be 0, got %d", list.scrollOffset)
	}

	// Move down to item 5 - should scroll to show 1-5
	list.SetSelection(5)
	if list.scrollOffset != 1 {
		t.Errorf("At item 5, scrollOffset should be 1 (showing 1-5), got %d", list.scrollOffset)
	}

	// Move down to item 6 - should scroll to show 2-6
	list.SetSelection(6)
	if list.scrollOffset != 2 {
		t.Errorf("At item 6, scrollOffset should be 2 (showing 2-6), got %d", list.scrollOffset)
	}

	// Move down to item 9 (last item) - should scroll to show 5-9
	list.SetSelection(9)
	if list.scrollOffset != 5 {
		t.Errorf("At item 9, scrollOffset should be 5 (showing 5-9), got %d", list.scrollOffset)
	}
}

// TestScrollingUpFromBottom tests the critical case: scrolling up from bottom
func TestScrollingUpFromBottom(t *testing.T) {
	// 10 items, each 5 units tall, viewport shows 5 items (height=25)
	list := createTestList(10, 5)
	setListHeight(list, 25) // Can show 5 items (25/5=5)

	// First scroll down to the bottom (item 9)
	list.SetSelection(9)
	if list.scrollOffset != 5 {
		t.Errorf("At item 9, scrollOffset should be 5 (showing 5-9), got %d", list.scrollOffset)
	}

	// Now press Up: move to item 8 - should NOT scroll, still showing 5-9
	list.SetSelection(8)
	if list.scrollOffset != 5 {
		t.Errorf("At item 8 (moved up from 9), scrollOffset should still be 5 (showing 5-9), got %d", list.scrollOffset)
	}

	// Press Up: move to item 7 - should NOT scroll, still showing 5-9
	list.SetSelection(7)
	if list.scrollOffset != 5 {
		t.Errorf("At item 7 (moved up from 8), scrollOffset should still be 5 (showing 5-9), got %d", list.scrollOffset)
	}

	// Press Up: move to item 6 - should NOT scroll, still showing 5-9
	list.SetSelection(6)
	if list.scrollOffset != 5 {
		t.Errorf("At item 6 (moved up from 7), scrollOffset should still be 5 (showing 5-9), got %d", list.scrollOffset)
	}

	// Press Up: move to item 5 - should NOT scroll, still showing 5-9
	list.SetSelection(5)
	if list.scrollOffset != 5 {
		t.Errorf("At item 5 (moved up from 6), scrollOffset should still be 5 (showing 5-9), got %d", list.scrollOffset)
	}

	// Press Up: move to item 4 - NOW should scroll to show 4-8
	list.SetSelection(4)
	if list.scrollOffset != 4 {
		t.Errorf("At item 4 (moved up from 5), scrollOffset should be 4 (showing 4-8), got %d", list.scrollOffset)
	}
}

// TestScrollingUpToTop tests scrolling all the way to the top
func TestScrollingUpToTop(t *testing.T) {
	// 10 items, each 5 units tall, viewport shows 5 items (height=25)
	list := createTestList(10, 5)
	setListHeight(list, 25)

	// Start at item 5 (middle)
	list.SetSelection(5)
	if list.scrollOffset != 1 {
		t.Errorf("At item 5, scrollOffset should be 1, got %d", list.scrollOffset)
	}

	// Move up to item 4
	list.SetSelection(4)
	if list.scrollOffset != 1 {
		t.Errorf("At item 4, scrollOffset should still be 1 (showing 1-5), got %d", list.scrollOffset)
	}

	// Move up to item 3
	list.SetSelection(3)
	if list.scrollOffset != 1 {
		t.Errorf("At item 3, scrollOffset should still be 1 (showing 1-5), got %d", list.scrollOffset)
	}

	// Move up to item 2
	list.SetSelection(2)
	if list.scrollOffset != 1 {
		t.Errorf("At item 2, scrollOffset should still be 1 (showing 1-5), got %d", list.scrollOffset)
	}

	// Move up to item 1 - still showing 1-5
	list.SetSelection(1)
	if list.scrollOffset != 1 {
		t.Errorf("At item 1, scrollOffset should still be 1 (showing 1-5), got %d", list.scrollOffset)
	}

	// Move up to item 0 - should scroll to show 0-4
	list.SetSelection(0)
	if list.scrollOffset != 0 {
		t.Errorf("At item 0, scrollOffset should be 0 (showing 0-4), got %d", list.scrollOffset)
	}
}

// TestScrollingDownThenUpComplete tests a full down-then-up cycle
func TestScrollingDownThenUpComplete(t *testing.T) {
	// 10 items, each 5 units tall, viewport shows 5 items (height=25)
	list := createTestList(10, 5)
	setListHeight(list, 25)

	// Scroll all the way down
	for i := 0; i <= 9; i++ {
		list.SetSelection(i)
	}
	if list.scrollOffset != 5 {
		t.Errorf("After scrolling to bottom, scrollOffset should be 5, got %d", list.scrollOffset)
	}

	// Now scroll all the way up
	expectedOffsets := []int{5, 5, 5, 5, 5, 4, 3, 2, 1, 0}
	for i := 9; i >= 0; i-- {
		list.SetSelection(i)
		expected := expectedOffsets[9-i]
		if list.scrollOffset != expected {
			t.Errorf("At item %d (scrolling up), scrollOffset should be %d, got %d", i, expected, list.scrollOffset)
		}
	}
}

// TestEdgeCaseFewerItemsThanViewport tests when there are fewer items than viewport can hold
func TestEdgeCaseFewerItemsThanViewport(t *testing.T) {
	// 3 items, viewport shows 5 items
	list := createTestList(3, 5)
	setListHeight(list, 25)

	// Move through all items - should never scroll
	list.SetSelection(0)
	if list.scrollOffset != 0 {
		t.Errorf("At item 0 with 3 items, scrollOffset should be 0, got %d", list.scrollOffset)
	}

	list.SetSelection(1)
	if list.scrollOffset != 0 {
		t.Errorf("At item 1 with 3 items, scrollOffset should be 0, got %d", list.scrollOffset)
	}

	list.SetSelection(2)
	if list.scrollOffset != 0 {
		t.Errorf("At item 2 with 3 items, scrollOffset should be 0, got %d", list.scrollOffset)
	}
}

// TestEdgeCaseExactFit tests when items exactly fill the viewport
func TestEdgeCaseExactFit(t *testing.T) {
	// 5 items, viewport shows exactly 5 items
	list := createTestList(5, 5)
	setListHeight(list, 25)

	// Should never need to scroll
	for i := 0; i < 5; i++ {
		list.SetSelection(i)
		if list.scrollOffset != 0 {
			t.Errorf("At item %d with exact fit, scrollOffset should be 0, got %d", i, list.scrollOffset)
		}
	}
}

// TestEdgeCaseOneMoreThanViewport tests the boundary case of viewport+1 items
func TestEdgeCaseOneMoreThanViewport(t *testing.T) {
	// 6 items, viewport shows 5 items
	list := createTestList(6, 5)
	setListHeight(list, 25)

	// Items 0-4 should not scroll
	for i := 0; i <= 4; i++ {
		list.SetSelection(i)
		if list.scrollOffset != 0 {
			t.Errorf("At item %d, scrollOffset should be 0, got %d", i, list.scrollOffset)
		}
	}

	// Item 5 should scroll by 1
	list.SetSelection(5)
	if list.scrollOffset != 1 {
		t.Errorf("At item 5, scrollOffset should be 1 (showing 1-5), got %d", list.scrollOffset)
	}

	// Move back up to 4 - should not scroll back yet
	list.SetSelection(4)
	if list.scrollOffset != 1 {
		t.Errorf("At item 4 (moved up from 5), scrollOffset should still be 1, got %d", list.scrollOffset)
	}

	// Move back up to 3
	list.SetSelection(3)
	if list.scrollOffset != 1 {
		t.Errorf("At item 3 (moved up from 4), scrollOffset should still be 1, got %d", list.scrollOffset)
	}

	// Move back up to 2
	list.SetSelection(2)
	if list.scrollOffset != 1 {
		t.Errorf("At item 2 (moved up from 3), scrollOffset should still be 1, got %d", list.scrollOffset)
	}

	// Move back up to 1 - still showing 1-5
	list.SetSelection(1)
	if list.scrollOffset != 1 {
		t.Errorf("At item 1 (moved up from 2), scrollOffset should still be 1, got %d", list.scrollOffset)
	}

	// Move back up to 0 - NOW should scroll to 0
	list.SetSelection(0)
	if list.scrollOffset != 0 {
		t.Errorf("At item 0 (moved up from 1), scrollOffset should be 0, got %d", list.scrollOffset)
	}
}

// TestRefreshCycle tests the pattern used in BoardView: Clear() + AddItem() + SetSelection()
func TestRefreshCycle(t *testing.T) {
	// 10 items, viewport shows 5
	list := createTestList(10, 5)
	setListHeight(list, 25)

	// Scroll to bottom
	list.SetSelection(9)
	if list.scrollOffset != 5 {
		t.Errorf("At item 9, scrollOffset should be 5, got %d", list.scrollOffset)
	}

	// Now simulate a refresh: Clear() + re-add items + SetSelection()
	// This is what BoardView.refresh() does
	oldScrollOffset := list.scrollOffset

	list.Clear() // Should preserve scrollOffset
	if list.scrollOffset != oldScrollOffset {
		t.Errorf("After Clear(), scrollOffset changed from %d to %d", oldScrollOffset, list.scrollOffset)
	}

	// Re-add items
	for i := 0; i < 10; i++ {
		list.AddItem(newMockPrimitive())
	}

	// Set selection to item 8 (moved up from 9)
	list.SetSelection(8)
	if list.scrollOffset != 5 {
		t.Errorf("After refresh with selection at 8, scrollOffset should still be 5 (showing 5-9), got %d", list.scrollOffset)
	}
}

// TestLargeItemHeight tests with different item heights
func TestLargeItemHeight(t *testing.T) {
	// 10 items, each 10 units tall, viewport shows 3 items (height=30)
	list := createTestList(10, 10)
	setListHeight(list, 30) // Can show 3 items (30/10=3)

	// Start at 0
	list.SetSelection(0)
	if list.scrollOffset != 0 {
		t.Errorf("At item 0, scrollOffset should be 0, got %d", list.scrollOffset)
	}

	// Move to item 2 (last visible) - should not scroll
	list.SetSelection(2)
	if list.scrollOffset != 0 {
		t.Errorf("At item 2, scrollOffset should be 0 (showing 0-2), got %d", list.scrollOffset)
	}

	// Move to item 3 - should scroll to show 1-3
	list.SetSelection(3)
	if list.scrollOffset != 1 {
		t.Errorf("At item 3, scrollOffset should be 1 (showing 1-3), got %d", list.scrollOffset)
	}

	// Move to item 9 (last) - should scroll to show 7-9
	list.SetSelection(9)
	if list.scrollOffset != 7 {
		t.Errorf("At item 9, scrollOffset should be 7 (showing 7-9), got %d", list.scrollOffset)
	}

	// Move up to 8 - should NOT scroll yet
	list.SetSelection(8)
	if list.scrollOffset != 7 {
		t.Errorf("At item 8 (moved up from 9), scrollOffset should still be 7 (showing 7-9), got %d", list.scrollOffset)
	}

	// Move up to 7 - should NOT scroll yet (showing 7-9)
	list.SetSelection(7)
	if list.scrollOffset != 7 {
		t.Errorf("At item 7 (moved up from 8), scrollOffset should still be 7 (showing 7-9), got %d", list.scrollOffset)
	}

	// Move up to 6 - NOW should scroll to show 6-8
	list.SetSelection(6)
	if list.scrollOffset != 6 {
		t.Errorf("At item 6 (moved up from 7), scrollOffset should be 6 (showing 6-8), got %d", list.scrollOffset)
	}
}

// TestGetScrollOffset tests the GetScrollOffset method
func TestGetScrollOffset(t *testing.T) {
	list := createTestList(10, 5)
	setListHeight(list, 25) // Can show 5 items

	// Initial scroll offset should be 0
	if offset := list.GetScrollOffset(); offset != 0 {
		t.Errorf("Initial GetScrollOffset() = %d, want 0", offset)
	}

	// After scrolling down, GetScrollOffset should reflect the internal state
	list.SetSelection(7) // Should scroll to show items 3-7
	if offset := list.GetScrollOffset(); offset != 3 {
		t.Errorf("After SetSelection(7), GetScrollOffset() = %d, want 3", offset)
	}

	// Scroll to bottom
	list.SetSelection(9) // Should scroll to show items 5-9
	if offset := list.GetScrollOffset(); offset != 5 {
		t.Errorf("After SetSelection(9), GetScrollOffset() = %d, want 5", offset)
	}

	// GetScrollOffset should match internal scrollOffset
	if list.GetScrollOffset() != list.scrollOffset {
		t.Errorf("GetScrollOffset() = %d doesn't match internal scrollOffset = %d",
			list.GetScrollOffset(), list.scrollOffset)
	}
}

func TestHiddenEntriesCountTikisNotRows(t *testing.T) {
	list := createTestList(10, 5) // 10 rows of 4, the last holding 2
	list.SetEntryCount(38, 4)

	cases := []struct{ offset, visible, above, below int }{
		{0, 3, 0, 26},
		{4, 3, 16, 10},
		{7, 3, 28, 0},
	}
	for _, c := range cases {
		list.scrollOffset = c.offset
		above, below := list.hiddenEntries(c.visible)
		if above != c.above || below != c.below {
			t.Errorf("offset %d, %d visible: hidden = (%d, %d), want (%d, %d)",
				c.offset, c.visible, above, below, c.above, c.below)
		}
	}
}

func TestHiddenEntriesDefaultToOnePerItem(t *testing.T) {
	list := createTestList(10, 5)
	list.scrollOffset = 2

	if above, below := list.hiddenEntries(5); above != 2 || below != 3 {
		t.Errorf("hidden = (%d, %d), want (2, 3)", above, below)
	}
}

func TestCapacityReservesHintLineOnlyForOverflowingOneLineItems(t *testing.T) {
	cases := []struct {
		name                            string
		items, itemHeight, height, want int
	}{
		{"one-line items that overflow give up a line", 10, 1, 5, 4},
		{"one-line items that fit keep every line", 5, 1, 5, 5},
		{"framed items carry hints in their borders", 10, 5, 25, 5},
		{"a lone line is never given up", 10, 1, 1, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			list := createTestList(c.items, c.itemHeight)
			if got := list.capacity(c.height); got != c.want {
				t.Errorf("capacity(%d) = %d, want %d", c.height, got, c.want)
			}
		})
	}
}

// newBorderedTestList builds 5-line framed items, like a 3-row board card.
func newBorderedTestList(itemCount int) *ScrollableList {
	list := NewScrollableList().SetItemHeight(5)
	for i := 0; i < itemCount; i++ {
		list.AddItem(tview.NewBox().SetBorder(true))
	}
	return list
}

func drawList(t *testing.T, list *ScrollableList, width, height int) tcell.Screen {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen init: %v", err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(width, height)
	list.SetRect(0, 0, width, height)
	list.Draw(screen)
	return screen
}

func TestFramedListDrawsHintsIntoEdgeBorders(t *testing.T) {
	list := newBorderedTestList(10)
	list.SetEntryCount(10, 1)
	list.SetRect(0, 0, 20, 15)
	list.SetSelection(5) // rows 3-5 visible

	screen := drawList(t, list, 20, 15)

	top := screenRow(screen, 0, 20)
	if top != "┌─────────▲ 3 more─┐" {
		t.Errorf("top border = %q, want the ▲ 3 more hint at its right end", top)
	}
	bottom := screenRow(screen, 14, 20)
	if bottom != "└─────────▼ 4 more─┘" {
		t.Errorf("bottom border = %q, want the ▼ 4 more hint at its right end", bottom)
	}

	_, hintStyle, _ := screen.Get(10, 0)
	_, cornerStyle, _ := screen.Get(0, 0)
	fg, bg, _ := hintStyle.Decompose()
	if want := theme.Roles().TextMuted().TCell(); fg != want {
		t.Errorf("hint foreground = %v, want muted text %v", fg, want)
	}
	if _, cornerBg, _ := cornerStyle.Decompose(); bg != cornerBg {
		t.Errorf("hint background = %v, want the border's %v", bg, cornerBg)
	}
}

func TestFramedListAtTopShowsOnlyBelowHint(t *testing.T) {
	list := newBorderedTestList(10)
	list.SetEntryCount(10, 1)

	screen := drawList(t, list, 20, 15)

	if top := screenRow(screen, 0, 20); strings.Contains(top, "▲") {
		t.Errorf("top border = %q, want no ▲ hint at the top of the list", top)
	}
	if bottom := screenRow(screen, 14, 20); bottom != "└─────────▼ 7 more─┘" {
		t.Errorf("bottom border = %q, want the ▼ 7 more hint", bottom)
	}
}

func TestListThatFitsShowsNoHints(t *testing.T) {
	list := newBorderedTestList(3)
	list.SetEntryCount(3, 1)

	screen := drawList(t, list, 20, 15)

	for y := 0; y < 15; y++ {
		if row := screenRow(screen, y, 20); strings.ContainsAny(row, "▲▼") {
			t.Errorf("row %d = %q, want no hints when everything fits", y, row)
		}
	}
}

func TestHintNeverCoversRightCorner(t *testing.T) {
	list := newBorderedTestList(10)
	list.SetEntryCount(10, 1)

	screen := drawList(t, list, 8, 15)

	if bottom := screenRow(screen, 14, 8); bottom != "└─▼ 7 ─┘" {
		t.Errorf("bottom border = %q, want the hint clipped before the corner", bottom)
	}
}

func TestOneLineListShowsStatusLine(t *testing.T) {
	cases := []struct {
		name      string
		selection int
		want      string
	}{
		{"at the top", 0, "▼ 6 more"},
		{"in the middle", 5, "▲ 2 · ▼ 4"},
		{"at the bottom", 9, "▲ 6 more"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			list := createTestList(10, 1)
			list.SetEntryCount(10, 1)
			list.SetSelection(c.selection)

			screen := drawList(t, list, 30, 5)

			if got := screenRow(screen, 4, 30); strings.TrimSpace(got) != c.want || !strings.HasSuffix(got, c.want) {
				t.Errorf("status line = %q, want %q at its right end", got, c.want)
			}
		})
	}
}
