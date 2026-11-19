package header

// layout constants for the header widget. kept here alongside the pure layout
// function so the algorithm and its inputs are co-located.
const (
	InfoWidth       = 40 // fixed width for info section (view name + description)
	MinContextWidth = 40 // minimum width for context help to remain readable
)

// HeaderLayout is the pure output of the header layout calculation.
// It contains no tview types — just the computed widths.
type HeaderLayout struct {
	ContextWidth int
	ShowLogo     bool
}

// CalculateHeaderLayout computes header component widths.
// The logo gives way to the key hints: it is shown only when the hints still get MinContextWidth,
// so a wide custom logo can't squeeze them away.
func CalculateHeaderLayout(totalWidth, contextHelpWidth, logoWidth int) HeaderLayout {
	showLogo := logoWidth > 0 && totalWidth >= InfoWidth+logoWidth+MinContextWidth
	reserved := InfoWidth
	if showLogo {
		reserved += logoWidth
	}
	availableBetween := max(totalWidth-reserved, 0)

	contextWidth := min(max(contextHelpWidth, 0), availableBetween)
	return HeaderLayout{ContextWidth: contextWidth, ShowLogo: showLogo}
}
