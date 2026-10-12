package palette

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/boolean-maybe/tiki/component"
	"github.com/boolean-maybe/tiki/model"
	"github.com/boolean-maybe/tiki/theme"
	tikipkg "github.com/boolean-maybe/tiki/tiki"
	"github.com/boolean-maybe/tiki/util"
)

const (
	confirmMaxListed = 5
	confirmMaxWidth  = 72
	// border plus horizontal padding on each side
	confirmChromeWidth = 6
	// border plus the blank row above the list
	confirmChromeHeight = 3
)

// ConfirmDialog asks before a delete action runs, listing the tikis it would remove.
type ConfirmDialog struct {
	view   *tview.TextView
	cfg    *model.ConfirmConfig
	width  int
	height int
}

// NewConfirmDialog creates the confirmation dialog.
func NewConfirmDialog(cfg *model.ConfirmConfig) *ConfirmDialog {
	roles := theme.Roles()
	tv := tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	tv.SetBackgroundColor(roles.SurfaceCanvas().TCell())
	tv.SetBorder(true)
	tv.SetBorderColor(roles.StatusDanger().TCell())
	tv.SetTitleColor(roles.StatusDanger().TCell())
	tv.SetBorderPadding(1, 0, 2, 2)

	d := &ConfirmDialog{view: tv, cfg: cfg}
	tv.SetInputCapture(d.handleKey)
	return d
}

// GetPrimitive returns the primitive that receives focus while the dialog is shown.
func (d *ConfirmDialog) GetPrimitive() tview.Primitive {
	return d.view
}

// Size returns the width and height the dialog needs for its current content.
func (d *ConfirmDialog) Size() (int, int) {
	return d.width, d.height
}

// NewConfirmOverlay returns a full-screen page that draws the dialog centred,
// leaving the view beneath it visible.
func NewConfirmOverlay(d *ConfirmDialog) tview.Primitive {
	return &confirmOverlay{Box: tview.NewBox(), dialog: d}
}

type confirmOverlay struct {
	*tview.Box
	dialog *ConfirmDialog
}

// tview's root Pages only passes keys to a page that reports focus, through that page's handler,
// so the overlay forwards focus and keys to the dialog.
func (o *confirmOverlay) HasFocus() bool {
	return o.dialog.GetPrimitive().HasFocus()
}

func (o *confirmOverlay) Focus(delegate func(p tview.Primitive)) {
	delegate(o.dialog.GetPrimitive())
}

func (o *confirmOverlay) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return o.dialog.GetPrimitive().InputHandler()
}

func (o *confirmOverlay) Draw(screen tcell.Screen) {
	x, y, w, h := o.GetRect()
	dw, dh := o.dialog.Size()
	dw, dh = min(dw, w), min(dh, h)
	p := o.dialog.GetPrimitive()
	p.SetRect(x+(w-dw)/2, y+(h-dh)/2, dw, dh)
	p.Draw(screen)
}

// Other keys are swallowed and leave the question open,
// so a stray key never leaves it unclear whether the tiki was deleted.
func (d *ConfirmDialog) handleKey(ev *tcell.EventKey) *tcell.EventKey {
	switch ev.Key() {
	case tcell.KeyEnter:
		d.cfg.Confirm()
	case tcell.KeyEscape:
		d.cfg.Cancel()
	}
	return nil
}

// OnShow fills the dialog with the tikis the delete would remove.
func (d *ConfirmDialog) OnShow(tikis []*tikipkg.Tiki) {
	roles := theme.Roles()
	title := "Delete tiki?"
	if len(tikis) != 1 {
		title = fmt.Sprintf("Delete %d tikis?", len(tikis))
	}
	d.view.SetTitle(" " + title + " ")

	listed := tikis
	if len(listed) > confirmMaxListed {
		listed = listed[:confirmMaxListed]
	}
	idWidth := component.ComputeIDColumnWidth(listed)
	titleWidth := confirmMaxWidth - confirmChromeWidth - idWidth - 2

	idHex := roles.TikiID().Hex()
	titleHex := roles.TextPrimary().Hex()
	mutedHex := roles.TextMuted().Hex()
	var lines []string
	for _, tk := range listed {
		lines = append(lines, fmt.Sprintf("[%s]%-*s[-]  [%s]%s[-]",
			idHex, idWidth, tk.ID(), titleHex, tview.Escape(util.TruncateText(tk.Title(), titleWidth))))
	}
	if more := len(tikis) - len(listed); more > 0 {
		lines = append(lines, fmt.Sprintf("[%s]…and %d more[-]", mutedHex, more))
	}
	lines = append(lines, "", fmt.Sprintf("[%s]Enter[-] [%s]delete[-]    [%s]Esc[-] [%s]cancel[-]",
		roles.StatusDanger().Hex(), mutedHex, roles.TextPrimary().Hex(), mutedHex))

	content := tview.TaggedStringWidth(title) + 2
	text := ""
	for i, line := range lines {
		content = max(content, tview.TaggedStringWidth(line))
		if i > 0 {
			text += "\n"
		}
		text += line
	}
	d.view.SetText(text)
	d.view.ScrollToBeginning()
	d.width = min(content+confirmChromeWidth, confirmMaxWidth)
	d.height = len(lines) + confirmChromeHeight
}
