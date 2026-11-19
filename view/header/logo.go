package header

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/boolean-maybe/tiki/theme"
	"github.com/rivo/tview"
)

// logoColors are the hex colours of the logo's three glyphs.
type logoColors struct {
	dot, shade, border string
}

func themeLogoColors(roles *theme.Theme) logoColors {
	return logoColors{dot: roles.LogoDot().Hex(), shade: roles.LogoShade().Hex(), border: roles.LogoBorder().Hex()}
}

// projectLogoColors derives the two darker tones from the project colour.
// Themes leave the background to the terminal, so there is no theme colour to blend toward:
// dark themes blend toward black, light themes toward white.
func projectLogoColors(hex string, light bool) logoColors {
	target := 0
	if light {
		target = 255
	}
	return logoColors{dot: hex, shade: mixHex(hex, target, 0.45), border: mixHex(hex, target, 0.7)}
}

func mixHex(hex string, target int, t float64) string {
	v, err := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	if err != nil {
		return hex
	}
	mix := func(c int) int { return c + int(float64(target-c)*t) }
	r, g, b := mix(int(v>>16&0xff)), mix(int(v>>8&0xff)), mix(int(v&0xff))
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// buildLogo colours art for tview and returns it with its width.
// ●, ▓ and ▒ take the three logo colours and everything else the dot colour.
// Text is escaped, so brackets in figlet output are never read as colour tags.
func buildLogo(art string, c logoColors) (string, int) {
	lines := strings.Split(art, "\n")
	if len(lines) > HeaderHeight {
		lines = lines[:HeaderHeight]
	}
	var b strings.Builder
	width := 0
	for _, line := range lines {
		width = max(width, tview.TaggedStringWidth(tview.Escape(line)))
		var run strings.Builder
		runColor := ""
		flush := func() {
			if run.Len() > 0 {
				fmt.Fprintf(&b, "[%s]%s", runColor, tview.Escape(run.String()))
				run.Reset()
			}
		}
		for _, r := range line {
			color := c.dot
			switch r {
			case '▓':
				color = c.shade
			case '▒':
				color = c.border
			}
			if color != runColor {
				flush()
				runColor = color
			}
			run.WriteRune(r)
		}
		flush()
		b.WriteString("[-]\n")
	}
	return b.String(), width
}
