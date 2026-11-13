package tikidetail

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boolean-maybe/tiki/controller"
	"github.com/boolean-maybe/tiki/store"
	"github.com/boolean-maybe/tiki/view/markdown"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const letteredOptionsBody = "The fix:\n" +
	"A. First option, wrapped by hand\n" +
	"   onto an indented line.\n" +
	"B. Second option."

func screenRows(t *testing.T, p tview.Primitive, width, height int) []string {
	t.Helper()
	rows := strings.Split(drawPrimitive(t, p, width, height), "\n")
	for i := range rows {
		rows[i] = strings.TrimRight(rows[i], " ")
	}
	return rows
}

// findRow returns the index of the first row holding sub and sub's column, or -1, -1.
func findRow(rows []string, sub string) (int, int) {
	for i, row := range rows {
		if col := strings.Index(row, sub); col >= 0 {
			return i, col
		}
	}
	return -1, -1
}

func assertLetteredOptionsKeptApart(t *testing.T, rows []string) {
	t.Helper()
	i, col := findRow(rows, "A. First option, wrapped by hand")
	if i < 0 || i+2 >= len(rows) {
		t.Fatalf("option A not on screen:\n%s", strings.Join(rows, "\n"))
	}
	pad := strings.Repeat(" ", col)
	want := []string{
		pad + "A. First option, wrapped by hand",
		pad + "   onto an indented line.",
		pad + "B. Second option.",
	}
	for j, w := range want {
		if rows[i+j] != w {
			t.Errorf("row %d = %q, want %q", i+j, rows[i+j], w)
		}
	}
}

func newBodyDetailView(t *testing.T, s store.Store, id string) *ConfigurableDetailView {
	t.Helper()
	cv := NewConfigurableDetailView(
		s, id,
		detailPluginFromFields([]string{"status"}),
		controller.DetailViewActions(),
		nil, nil,
		nil, nil,
	)
	cv.EnterFullscreen()
	return cv
}

func createTiki(t *testing.T, s store.Store, id, body string) {
	t.Helper()
	tk := newTestViewTiki(id)
	tk.SetBody(body)
	if err := s.CreateTiki(tk); err != nil {
		t.Fatalf("CreateTiki(%s): %v", id, err)
	}
}

func followFirstLink(t *testing.T, cv *ConfigurableDetailView) {
	t.Helper()
	// the first draw gives the viewer its width, which places the links
	screenRows(t, cv.GetPrimitive(), 80, 20)
	handler := cv.navMarkdown.Viewer().InputHandler()
	for _, key := range []tcell.Key{tcell.KeyTab, tcell.KeyEnter} {
		handler(tcell.NewEventKey(key, 0, tcell.ModNone), func(tview.Primitive) {})
	}
}

func TestDetailBody_KeepsTypedLineBreaksAndIndent(t *testing.T) {
	s := store.NewInMemoryStore()
	createTiki(t, s, "AAAAAA", letteredOptionsBody)

	cv := newBodyDetailView(t, s, "AAAAAA")
	assertLetteredOptionsKeptApart(t, screenRows(t, cv.GetPrimitive(), 80, 20))
}

func TestDetailBody_LinkedTicketKeepsLineBreaks(t *testing.T) {
	s := store.NewInMemoryStore()
	createTiki(t, s, "AAAAAA", letteredOptionsBody)
	createTiki(t, s, "BBBBBB", "See [[AAAAAA]] for the options.")

	cv := newBodyDetailView(t, s, "BBBBBB")
	followFirstLink(t, cv)
	assertLetteredOptionsKeptApart(t, screenRows(t, cv.GetPrimitive(), 80, 20))
}

func TestDetailBody_LinkedDocStillJoinsLines(t *testing.T) {
	doc := filepath.Join(t.TempDir(), "notes.md")
	if err := os.WriteFile(doc, []byte(letteredOptionsBody+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := store.NewInMemoryStore()
	createTiki(t, s, "BBBBBB", "See [the notes]("+doc+").")

	cv := newBodyDetailView(t, s, "BBBBBB")
	followFirstLink(t, cv)
	rows := screenRows(t, cv.GetPrimitive(), 80, 20)
	if i, _ := findRow(rows, "The fix: A. First option, wrapped by hand onto an indented line."); i < 0 {
		t.Errorf("doc page should render as one paragraph:\n%s", strings.Join(rows, "\n"))
	}
}

func TestNavigableMarkdown_JoinsLinesUnlessAsked(t *testing.T) {
	nm := markdown.NewNavigableMarkdown(markdown.NavigableMarkdownConfig{})
	nm.SetMarkdown(letteredOptionsBody)
	rows := screenRows(t, nm.Viewer(), 80, 20)
	if i, _ := findRow(rows, "The fix: A. First option, wrapped by hand onto an indented line."); i < 0 {
		t.Errorf("default viewer should render one paragraph:\n%s", strings.Join(rows, "\n"))
	}
}

func TestIsTikiSource(t *testing.T) {
	s := store.NewInMemoryStore()
	createTiki(t, s, "AAAAAA", "body")

	tests := []struct {
		source string
		want   bool
	}{
		{"/tickets/own.md", true},
		{"AAAAAA", true},
		{"aaaaaa", true},
		{"/tickets/other.md", false},
		{"ZZZZZZ", false},
		{"/docs/notes.md", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isTikiSource(s, "/tickets/own.md", tt.source); got != tt.want {
			t.Errorf("isTikiSource(%q) = %v, want %v", tt.source, got, tt.want)
		}
	}
	if isTikiSource(s, "", "") {
		t.Error("an empty source must not match an empty own path")
	}
}

// Link text is never split by a typed break: a link drawn over two rows cannot be selected.
func TestDetailBody_LinkTypedAcrossTwoLinesCanBeFollowed(t *testing.T) {
	doc := filepath.Join(t.TempDir(), "notes.md")
	if err := os.WriteFile(doc, []byte("Reached the notes page.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := store.NewInMemoryStore()
	createTiki(t, s, "BBBBBB", "See [the\nnotes]("+doc+") now.")

	cv := newBodyDetailView(t, s, "BBBBBB")
	followFirstLink(t, cv)
	rows := screenRows(t, cv.GetPrimitive(), 80, 20)
	if i, _ := findRow(rows, "Reached the notes page."); i < 0 {
		t.Errorf("the link was not followed:\n%s", strings.Join(rows, "\n"))
	}
}
