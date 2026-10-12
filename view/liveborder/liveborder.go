// Package liveborder draws a tiki's live status into the bottom border of its box,
// on board cards and in the detail view alike.
package liveborder

import (
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/rivo/uniseg"

	"github.com/boolean-maybe/tiki/internal/livestatus"
	"github.com/boolean-maybe/tiki/theme"
)

type Source interface {
	Line(id string) (livestatus.Line, bool)
	// Drawn keeps redraws coming while statuses are on screen.
	Drawn(next time.Duration)
}

// Frame draws into the border rather than a row of its own,
// so boxes keep one height whether or not an agent is working on the tiki.
type Frame struct {
	*tview.Frame
	id    string
	live  Source
	roles *theme.Theme
	limit int
}

func New(frame *tview.Frame, id string, live Source, roles *theme.Theme) *Frame {
	return &Frame{Frame: frame, id: id, live: live, roles: roles}
}

// The overflow hint is drawn into the same border,
// so the status stops before screen column x.
// x <= 0 removes the limit.
func (c *Frame) SetBottomBorderLimit(x int) { c.limit = x }

func (c *Frame) Draw(screen tcell.Screen) {
	c.Frame.Draw(screen)
	if c.live == nil {
		return
	}
	line, ok := c.live.Line(c.id)
	if !ok {
		return
	}
	x, y, width, height := c.GetRect()
	// two cells in from each corner, like the overflow hints
	start, end := x+2, x+width-2
	if c.limit > 0 {
		end = min(end, c.limit)
	}
	if height < 2 || paintLiveLine(screen, start, end, y+height-1, line, c.roles) == 0 {
		return
	}
	c.live.Drawn(line.Next)
}

type liveCell struct {
	str   string
	width int
	color tcell.Color
}

type livePart struct {
	text  string
	color tcell.Color
}

// A message cut shorter than this says too little to be worth the space.
const minLiveMessageCells = 5

// paintLiveLine keeps each cell's background, like putHint,
// so the status doesn't punch a hole in the box's colour.
// A space on each side keeps the status off the border line, as with a box title.
func paintLiveLine(screen tcell.Screen, start, end, y int, line livestatus.Line, roles *theme.Theme) int {
	cells := fitLiveLine(line, roles, end-start-2)
	if len(cells) == 0 {
		return 0
	}
	pad := liveCell{str: " ", width: 1, color: roles.TextMuted().TCell()}
	cells = append(append([]liveCell{pad}, cells...), pad)
	x := start
	for _, c := range cells {
		_, style, _ := screen.Get(x, y)
		screen.Put(x, y, c.str, style.Foreground(c.color))
		x += c.width
	}
	return x - start
}

// fitLiveLine gives way part by part rather than cutting from the right:
// the message says the most, and a cut age would read as a different value.
func fitLiveLine(line livestatus.Line, roles *theme.Theme, avail int) []liveCell {
	accent, muted, faded := roles.LiveAgent().TCell(), roles.TextMuted().TCell(), roles.LiveMessage().TCell()
	if line.Stale {
		accent, faded = muted, muted
	}
	agent := livePart{line.Agent, accent}
	glyph := livePart{line.Glyph, accent}
	age := livePart{line.Age, muted}
	message := livePart{line.Message, faded}

	for _, lead := range [][]livePart{
		{agent, glyph, age},
		{glyph, age},
		{glyph},
	} {
		if cells, ok := fitLiveMessage(lead, message, avail, muted); ok {
			return cells
		}
	}
	if cells := partCells(nil, glyph, muted); cellsWidth(cells) <= avail {
		return cells
	}
	return nil
}

func fitLiveMessage(lead []livePart, message livePart, avail int, sep tcell.Color) ([]liveCell, bool) {
	var cells []liveCell
	for _, p := range lead {
		cells = partCells(cells, p, sep)
	}
	gap := 0
	if len(cells) > 0 {
		gap = 1
	}
	room := avail - cellsWidth(cells) - gap
	full := partCells(nil, message, sep)
	if cellsWidth(full) <= room {
		return partCells(cells, message, sep), true
	}
	if room < minLiveMessageCells {
		return nil, false
	}
	if gap > 0 {
		cells = append(cells, liveCell{str: " ", width: 1, color: sep})
	}
	used := 0
	for _, c := range full {
		if used+c.width > room-1 {
			break
		}
		cells = append(cells, c)
		used += c.width
	}
	return append(cells, liveCell{str: "…", width: 1, color: message.color}), true
}

func partCells(cells []liveCell, p livePart, sep tcell.Color) []liveCell {
	if p.text == "" {
		return cells
	}
	if len(cells) > 0 {
		cells = append(cells, liveCell{str: " ", width: 1, color: sep})
	}
	g := uniseg.NewGraphemes(p.text)
	for g.Next() {
		cells = append(cells, liveCell{str: g.Str(), width: uniseg.StringWidth(g.Str()), color: p.color})
	}
	return cells
}

func cellsWidth(cells []liveCell) int {
	w := 0
	for _, c := range cells {
		w += c.width
	}
	return w
}
