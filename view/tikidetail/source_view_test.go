package tikidetail

import (
	"strings"
	"testing"

	"github.com/boolean-maybe/tiki/theme"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/spf13/viper"
)

// drawnCell is one screen cell: its text and foreground colour.
type drawnCell struct {
	text string
	fg   tcell.Color
}

func drawCells(t *testing.T, p tview.Primitive, width, height int) [][]drawnCell {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(width, height)
	p.SetRect(0, 0, width, height)
	p.Draw(screen)

	rows := make([][]drawnCell, height)
	for y := range rows {
		rows[y] = make([]drawnCell, width)
		for x := range rows[y] {
			str, style, _ := screen.Get(x, y)
			fg, _, _ := style.Decompose()
			rows[y][x] = drawnCell{text: str, fg: fg}
		}
	}
	return rows
}

func cellRowText(row []drawnCell) string {
	var b strings.Builder
	for _, c := range row {
		if c.text == "" {
			b.WriteByte(' ')
			continue
		}
		b.WriteString(c.text)
	}
	return strings.TrimRight(b.String(), " ")
}

// fgOf returns the foreground of the first cell of sub on the screen.
func fgOf(t *testing.T, cells [][]drawnCell, sub string) tcell.Color {
	t.Helper()
	for _, row := range cells {
		if col := strings.Index(cellRowText(row), sub); col >= 0 {
			return row[len([]rune(cellRowText(row)[:col]))].fg
		}
	}
	t.Fatalf("%q not on screen", sub)
	return tcell.ColorDefault
}

func TestSourceView_ShowsBracketTextVerbatim(t *testing.T) {
	body := `plain [red] and [::b] and ["r"] and [[ABC123]] and [-] end`
	rows := screenRows(t, newSourceView(body), 80, 6)
	if i, _ := findRow(rows, body); i < 0 {
		t.Errorf("bracket text altered:\n%s", strings.Join(rows, "\n"))
	}
}

func TestSourceView_KeepsLayoutAsTyped(t *testing.T) {
	rows := screenRows(t, newSourceView("# Title\n\n"+letteredOptionsBody+"\n\n- **bold** item"), 80, 12)
	i, col := findRow(rows, "# Title")
	if i < 0 {
		t.Fatalf("heading markup not shown:\n%s", strings.Join(rows, "\n"))
	}
	pad := strings.Repeat(" ", col)
	want := []string{
		pad + "# Title",
		"",
		pad + "The fix:",
		pad + "A. First option, wrapped by hand",
		pad + "   onto an indented line.",
		pad + "B. Second option.",
		"",
		pad + "- **bold** item",
	}
	for j, w := range want {
		if rows[i+j] != w {
			t.Errorf("row %d = %q, want %q", i+j, rows[i+j], w)
		}
	}
}

func TestSourceView_ColoursMarkupAndUsesThemeTextColour(t *testing.T) {
	cells := drawCells(t, newSourceView("plain words\n\n# Title"), 60, 8)
	plain := fgOf(t, cells, "plain words")
	if want := theme.Roles().TextPrimary().TCell(); plain != want {
		t.Errorf("plain text fg = %v, want the theme's primary text %v", plain, want)
	}
	if heading := fgOf(t, cells, "# Title"); heading == plain {
		t.Error("a heading should be coloured differently from plain text")
	}
}

func TestSourceView_WrapsLongLines(t *testing.T) {
	body := strings.TrimSpace(strings.Repeat("word ", 40))
	rows := screenRows(t, newSourceView(body), 40, 12)
	words := 0
	for _, row := range rows {
		if len([]rune(row)) > 40 {
			t.Errorf("row wider than the pane: %q", row)
		}
		words += strings.Count(row, "word")
	}
	if words != 40 {
		t.Errorf("%d of 40 words on screen", words)
	}
}

func TestSourceView_EmptyBodyShowsPlaceholder(t *testing.T) {
	rows := screenRows(t, newSourceView(""), 40, 5)
	if i, _ := findRow(rows, "(No description)"); i < 0 {
		t.Errorf("placeholder missing:\n%s", strings.Join(rows, "\n"))
	}
}

func TestSourceView_UnknownCodeThemeStillRenders(t *testing.T) {
	viper.Set("appearance.codeBlock.theme", "no-such-style")
	t.Cleanup(func() { viper.Set("appearance.codeBlock.theme", "") })

	rows := screenRows(t, newSourceView("# Title\nbody"), 40, 6)
	if i, _ := findRow(rows, "# Title"); i < 0 {
		t.Errorf("source not shown:\n%s", strings.Join(rows, "\n"))
	}
}

func TestHighlightSource_NoTrailingNewline(t *testing.T) {
	if out := highlightSource("one\ntwo"); strings.HasSuffix(out, "\n") {
		t.Errorf("output ends with a newline the body does not have: %q", out)
	}
}

// sourceRows draws body wide enough that nothing wraps and returns the body's rows as drawn.
func sourceRows(t *testing.T, body string) []string {
	t.Helper()
	lines := strings.Split(body, "\n")
	width := 8
	for _, l := range lines {
		width = max(width, len([]rune(l))+8)
	}
	rows := screenRows(t, newSourceView(body), width, len(lines)+2)
	// border padding: one row above, two columns to the left
	out := make([]string, len(lines))
	for i := range lines {
		out[i] = strings.TrimPrefix(rows[i+1], "  ")
	}
	return out
}

func assertShownAsTyped(t *testing.T, body string) {
	t.Helper()
	want := strings.Split(body, "\n")
	for i := range want {
		want[i] = strings.TrimRight(want[i], " ")
	}
	got := sourceRows(t, body)
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("body %q\n row %d = %q\n   want %q", body, i, got[i], want[i])
			return
		}
	}
}

func TestSourceView_ShowsTagLikeTextAsTyped(t *testing.T) {
	bodies := []string{
		"[:::https://example.com/x?y=1]after",
		"see [:::/] here",
		"[red:blue:b:http://x.y/z]text",
		"[INFO:app:b:msg]",
		"[::b:c]",
		"[:::]",
		"[::: todo]",
		"[-:-:-:x]",
		"[:-:-:http://x/?q=1&r=2]",
		`{"items": [{"tags": []}]}`,
		"x = [y + []]",
		"m[f(x)[]]",
		"[\n  {\"tags\": []}\n]",
		"```\n[red] {\"a\": []}\n```",
		"`[red]` and `x[]`",
		"    [::b] indented code []",
		"[red[] already escaped [red[[]",
		"[\"region\"] and [\"\"]",
		"[[[]]] ][ []",
	}
	for _, body := range bodies {
		assertShownAsTyped(t, body)
	}
}

func TestSourceView_KeepsLinesThatStartWithADash(t *testing.T) {
	for _, body := range []string{
		"Scores:\n-1 bad\n+1 good",
		"diff:\n-1,3 removed",
		"- item\n-1",
		"a\n\n-1",
	} {
		assertShownAsTyped(t, body)
	}
}

// Every short string over the characters tview's tag syntax is made of must come out as typed.
func TestSourceView_ShowsAnyTagSyntaxAsTyped(t *testing.T) {
	const alphabet = "[]:-#\"ab1 \n"
	state := uint32(1)
	next := func(n int) int {
		state = state*1664525 + 1013904223
		return int(state>>16) % n
	}
	for range 3000 {
		var b strings.Builder
		for range 1 + next(14) {
			b.WriteByte(alphabet[next(len(alphabet))])
		}
		body := strings.Trim(b.String(), " \n")
		if body == "" {
			continue
		}
		assertShownAsTyped(t, body)
		if t.Failed() {
			return
		}
	}
}

func TestSourceView_AttributesDoNotLeakIntoTheNextRun(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(60, 6)
	v := newSourceView("# Title\nplain after")
	v.SetRect(0, 0, 60, 6)
	v.Draw(screen)

	attrsAt := func(x, y int) tcell.AttrMask {
		_, style, _ := screen.Get(x, y)
		_, _, attrs := style.Decompose()
		return attrs
	}
	if attrsAt(2, 1)&tcell.AttrBold == 0 {
		t.Skip("the code theme does not draw headings bold")
	}
	if attrsAt(2, 2)&tcell.AttrBold != 0 {
		t.Error("plain text after a bold heading is drawn bold")
	}
}
