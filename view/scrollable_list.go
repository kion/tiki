package view

import (
	"fmt"
	"unicode/utf8"

	"github.com/boolean-maybe/tiki/theme"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ScrollableList is a container that displays a list of primitives and handles vertical scrolling.
// It ensures that the selected item is always visible.
type ScrollableList struct {
	*tview.Box

	items          []tview.Primitive
	itemHeight     int
	scrollOffset   int
	selectionIndex int
	entries        int
	perRow         int
}

// NewScrollableList creates a new scrollable list container
func NewScrollableList() *ScrollableList {
	return &ScrollableList{
		Box:            tview.NewBox(),
		items:          make([]tview.Primitive, 0),
		itemHeight:     1, // default, should be set by caller
		scrollOffset:   0,
		selectionIndex: -1,
		perRow:         1,
	}
}

// SetItemHeight sets the height of each item in the list
func (s *ScrollableList) SetItemHeight(height int) *ScrollableList {
	s.itemHeight = height
	return s
}

// AddItem adds a primitive to the list
func (s *ScrollableList) AddItem(item tview.Primitive) *ScrollableList {
	s.items = append(s.items, item)
	return s
}

// Clear removes all items from the list
func (s *ScrollableList) Clear() *ScrollableList {
	s.items = make([]tview.Primitive, 0)
	// Keep scrollOffset to preserve position during refresh
	s.selectionIndex = -1
	s.entries = 0
	s.perRow = 1
	return s
}

// SetEntryCount makes the overflow hints count entries rather than items,
// for items that each hold up to perRow entries.
func (s *ScrollableList) SetEntryCount(total, perRow int) *ScrollableList {
	s.entries = total
	s.perRow = max(perRow, 1)
	return s
}

// SetSelection sets the index of the selected item and scrolls to keep it visible
func (s *ScrollableList) SetSelection(index int) {
	s.selectionIndex = index
	s.ensureSelectionVisible()
}

// GetScrollOffset returns the current scroll offset (first visible item index)
func (s *ScrollableList) GetScrollOffset() int {
	return s.scrollOffset
}

// ResetScrollOffset resets the scroll offset to 0
func (s *ScrollableList) ResetScrollOffset() {
	s.scrollOffset = 0
}

// ensureSelectionVisible adjusts scrollOffset to keep selectionIndex in view
func (s *ScrollableList) ensureSelectionVisible() {
	// If no items, preserve scrollOffset (will be adjusted after items are added)
	if len(s.items) == 0 {
		return
	}

	// Ensure scrollOffset is within valid bounds
	if s.scrollOffset >= len(s.items) {
		s.scrollOffset = len(s.items) - 1
	}
	if s.scrollOffset < 0 {
		s.scrollOffset = 0
	}

	if s.selectionIndex < 0 {
		return
	}

	// Calculate view dimensions
	_, _, _, height := s.GetInnerRect()
	if height <= 0 {
		return
	}

	maxVisible := s.capacity(height)
	if maxVisible <= 0 {
		return
	}

	// Calculate the last visible index
	lastVisibleIndex := s.scrollOffset + maxVisible - 1
	if lastVisibleIndex >= len(s.items) {
		lastVisibleIndex = len(s.items) - 1
	}

	// Adjust scroll offset if selection is out of view
	// When scrolling up: only adjust when selection goes ABOVE the first visible item
	if s.selectionIndex < s.scrollOffset {
		s.scrollOffset = s.selectionIndex
	} else if s.selectionIndex > lastVisibleIndex {
		// When scrolling down: adjust to show the selected item at the bottom
		s.scrollOffset = s.selectionIndex - maxVisible + 1
	}

	// Ensure valid bounds for scrollOffset
	if s.scrollOffset < 0 {
		s.scrollOffset = 0
	}
	maxScrollOffset := len(s.items) - maxVisible
	if maxScrollOffset < 0 {
		maxScrollOffset = 0
	}
	if s.scrollOffset > maxScrollOffset {
		s.scrollOffset = maxScrollOffset
	}
}

// Draw draws this primitive onto the screen
func (s *ScrollableList) Draw(screen tcell.Screen) {
	s.DrawForSubclass(screen, s)

	x, y, width, height := s.GetInnerRect()
	if height <= 0 || width <= 0 {
		return
	}

	// Re-run scroll calculation in case height changed (resize)
	s.ensureSelectionVisible()

	maxVisible := s.capacity(height)

	// Loop through visible items
	for i := 0; i < maxVisible; i++ {
		itemIndex := s.scrollOffset + i
		if itemIndex >= len(s.items) {
			break
		}

		item := s.items[itemIndex]

		// set position and size for the item
		itemY := y + (i * s.itemHeight)
		item.SetRect(x, itemY, width, s.itemHeight)

		// draw the item
		item.Draw(screen)
	}

	s.drawHints(screen, x, y, width, height, maxVisible)
}

func (s *ScrollableList) capacity(height int) int {
	n := height / s.itemHeight
	// One-line items have no border to carry the overflow hints,
	// so an overflowing list gives its last line to them instead.
	if s.itemHeight == 1 && len(s.items) > n && n > 1 {
		n--
	}
	return n
}

func (s *ScrollableList) hiddenEntries(visible int) (above, below int) {
	total := s.entries
	if total == 0 {
		total = len(s.items) * s.perRow
	}
	shown := min(s.scrollOffset+visible, len(s.items))
	return s.scrollOffset * s.perRow, max(total-shown*s.perRow, 0)
}

func (s *ScrollableList) drawHints(screen tcell.Screen, x, y, width, height, visible int) {
	if visible <= 0 {
		return
	}
	above, below := s.hiddenEntries(visible)
	if above == 0 && below == 0 {
		return
	}
	fg := theme.Roles().TextMuted().TCell()

	// Hints sit at the right end because items are busiest on the left.
	if s.itemHeight == 1 {
		if visible < height {
			putHint(screen, x, x+width, y+visible, statusLine(above, below), fg)
		}
		return
	}
	// two cells in from each side keeps a dash between a hint and the border corners
	if above > 0 {
		putHint(screen, x+2, x+width-2, y, fmt.Sprintf("▲ %d more", above), fg)
	}
	if below > 0 {
		putHint(screen, x+2, x+width-2, y+visible*s.itemHeight-1, fmt.Sprintf("▼ %d more", below), fg)
	}
}

func statusLine(above, below int) string {
	switch {
	case above > 0 && below > 0:
		return fmt.Sprintf("▲ %d · ▼ %d", above, below)
	case above > 0:
		return fmt.Sprintf("▲ %d more", above)
	default:
		return fmt.Sprintf("▼ %d more", below)
	}
}

// putHint keeps each cell's background,
// so a hint over a focused or selected item doesn't punch a hole in its colour.
func putHint(screen tcell.Screen, start, end, y int, text string, fg tcell.Color) {
	x := max(end-utf8.RuneCountInString(text), start)
	for _, r := range text {
		if x >= end {
			return
		}
		_, style, _ := screen.Get(x, y)
		screen.SetContent(x, y, r, nil, style.Foreground(fg))
		x++
	}
}

// Focus is called when this primitive receives focus
func (s *ScrollableList) Focus(delegate func(p tview.Primitive)) {
	// We don't necessarily need to pass focus to children if they are just visual boxes.
	// But if children need focus (e.g. if they had buttons), we would need to manage that.
	// For now, the Board/Backlog handle input at the controller level, and these are just display.
	// So we might not strictly need to delegate focus, but it's good practice if we want children to draw focused styles.

	// In this specific use case (TikiBox), the "selected" state is passed in during creation/refresh
	// via border color changes. The TikiBox itself doesn't handle input or focus events in the tview sense.
	// So we can just let the Box handle focus for now.
	s.Box.Focus(delegate)
}
