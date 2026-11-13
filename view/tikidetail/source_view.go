package tikidetail

import (
	"strings"

	"github.com/boolean-maybe/tiki/config"
	"github.com/boolean-maybe/tiki/theme"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/rivo/tview"
)

const noDescription = "(No description)"

// newSourceView shows a tiki body as typed, with markdown syntax colouring.
func newSourceView(body string) *tview.TextView {
	roles := theme.Roles()
	tv := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetWordWrap(true).
		SetScrollable(true)
	tv.SetBorderPadding(1, 1, 2, 2)
	tv.SetBackgroundColor(roles.SurfaceCanvas().TCell())
	tv.SetTextColor(roles.TextPrimary().TCell())
	if body == "" {
		tv.SetText(roles.TextMuted().Tag() + noDescription)
		return tv
	}
	tv.SetText(highlightSource(body))
	return tv
}

// highlightSource returns body as tview-tagged text coloured by chroma's
// markdown lexer, in the style used for code blocks.
func highlightSource(body string) string {
	plainColour := theme.Roles().TextPrimary().Tag()
	plain := plainColour + attrTag(false, false, false)
	lexer := lexers.Get("markdown")
	if lexer == nil {
		return plain + literal(body, plain)
	}
	// the lexer's line rules (headings, list items) only match a line that ends in a newline,
	// and a tiki body is stored without a trailing one
	added := !strings.HasSuffix(body, "\n")
	src := body
	if added {
		src += "\n"
	}
	tokens, err := lexer.Tokenise(nil, src)
	if err != nil {
		return plain + literal(body, plain)
	}

	style := styles.Get(config.GetCodeBlockTheme())
	// a token left at the style's own text colour follows the app theme instead,
	// because chroma styles assume their own background
	base := style.Get(chroma.Text).Colour

	type styledRun struct{ tag, text string }
	var runs []styledRun
	for tok := tokens(); tok != chroma.EOF; tok = tokens() {
		tag := sourceTag(style.Get(tok.Type), base, plainColour)
		if n := len(runs); n > 0 && runs[n-1].tag == tag {
			runs[n-1].text += tok.Value
			continue
		}
		runs = append(runs, styledRun{tag: tag, text: tok.Value})
	}
	if n := len(runs); added && n > 0 {
		runs[n-1].text = strings.TrimSuffix(runs[n-1].text, "\n")
	}

	var out strings.Builder
	for _, r := range runs {
		out.WriteString(r.tag)
		out.WriteString(literal(r.text, r.tag))
	}
	return out.String()
}

// literal makes text safe from tview's tag parser without changing what is drawn.
// tview.Escape is not enough: it misses tags with a URL field and text that already looks escaped.
// Repeating the current style tag right after every "[" works for any text,
// because a "[" followed by "[" can open neither a tag nor an escape.
// The tag after every newline keeps tview from joining a line that starts with "-1" to the one above.
func literal(text, tag string) string {
	return strings.NewReplacer("[", "["+tag, "\n", "\n"+tag).Replace(text)
}

func sourceTag(entry chroma.StyleEntry, base chroma.Colour, plainColour string) string {
	colour := plainColour
	if entry.Colour.IsSet() && entry.Colour != base {
		colour = "[" + entry.Colour.String() + "]"
	}
	return colour + attrTag(entry.Bold == chroma.Yes, entry.Italic == chroma.Yes, entry.Underline == chroma.Yes)
}

// attrTag sets each attribute on or off,
// because tview adds a tag's attributes to the ones already on.
func attrTag(bold, italic, underline bool) string {
	flag := func(on bool, set, clear byte) byte {
		if on {
			return set
		}
		return clear
	}
	return "[::" + string([]byte{flag(bold, 'b', 'B'), flag(italic, 'i', 'I'), flag(underline, 'u', 'U')}) + "]"
}
