package header

import (
	"fmt"
	"strings"

	"github.com/boolean-maybe/tiki/theme"

	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

// infoTextWidth is InfoWidth minus the 1-column left padding.
const infoTextWidth = InfoWidth - 1

const titleSeparator = " • "

// InfoWidget displays the project name, the current view name and its description in the header
type InfoWidget struct {
	*tview.TextView
	project      string
	projectColor string
}

// NewInfoWidget creates a new info display widget
func NewInfoWidget() *InfoWidget {
	tv := tview.NewTextView()
	tv.SetDynamicColors(true)
	tv.SetTextAlign(tview.AlignLeft)
	tv.SetWrap(true)
	tv.SetWordWrap(true)
	tv.SetBorderPadding(0, 0, 1, 0)

	return &InfoWidget{TextView: tv}
}

// SetProject sets the project name shown before the view name, and its colour as "#rrggbb".
func (iw *InfoWidget) SetProject(name, colorHex string) {
	iw.project = name
	iw.projectColor = colorHex
}

// SetViewInfo updates the displayed view name and description
func (iw *InfoWidget) SetViewInfo(name, description string) {
	roles := theme.Roles()

	boldColor := roles.StatusWarn().BoldTag()
	mutedTag := roles.TextMuted().Tag()

	var title strings.Builder
	if project := fitTitle(iw.project, name, infoTextWidth); project != "" {
		fmt.Fprintf(&title, "[%s::b]%s[-::-]%s%s[-]", iw.projectColor, tview.Escape(project), mutedTag, titleSeparator)
	}
	fmt.Fprintf(&title, "%s%s[-::-]", boldColor, tview.Escape(name))

	text := title.String()
	if description != "" {
		separator := strings.Repeat("─", infoTextWidth)
		text = fmt.Sprintf("%s\n%s%s[-]\n%s%s", text, mutedTag, separator, mutedTag, description)
	}

	iw.SetText(text)
}

// fitTitle returns the part of the project name that fits before the view name on one line.
// The widget word-wraps, so an over-long line would wrap instead of being cut:
// the project name is truncated, and dropped when the view name alone fills the line.
func fitTitle(project, view string, width int) string {
	if project == "" {
		return ""
	}
	avail := width - runewidth.StringWidth(view) - runewidth.StringWidth(titleSeparator)
	if runewidth.StringWidth(project) <= avail {
		return project
	}
	if avail < 2 {
		return ""
	}
	return runewidth.Truncate(project, avail, "…")
}

// Primitive returns the underlying tview primitive
func (iw *InfoWidget) Primitive() tview.Primitive {
	return iw.TextView
}
