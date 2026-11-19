package header

import (
	"strings"
	"testing"

	"github.com/boolean-maybe/tiki/config"
	"github.com/rivo/tview"
)

var testLogoColors = logoColors{dot: "#111111", shade: "#222222", border: "#333333"}

func TestBuildLogo_DefaultArtWidth(t *testing.T) {
	_, width := buildLogo(config.DefaultLogoArt, testLogoColors)
	if width != 25 {
		t.Errorf("width = %d, want 25", width)
	}
}

func TestBuildLogo_GlyphColours(t *testing.T) {
	text, _ := buildLogo("●▓▒x", testLogoColors)
	for _, want := range []string{"[#111111]●", "[#222222]▓", "[#333333]▒", "[#111111]x"} {
		if !strings.Contains(text, want) {
			t.Errorf("logo %q is missing %q", text, want)
		}
	}
}

func TestBuildLogo_EscapesTags(t *testing.T) {
	text, width := buildLogo("[red]hi", testLogoColors)
	tv := tview.NewTextView().SetDynamicColors(true)
	tv.SetText(text)
	if got := strings.TrimRight(tv.GetText(true), "\n"); got != "[red]hi" {
		t.Errorf("rendered = %q, want the brackets shown literally", got)
	}
	if width != 7 {
		t.Errorf("width = %d, want 7", width)
	}
}

func TestBuildLogo_CutsToHeaderHeight(t *testing.T) {
	text, _ := buildLogo("1\n2\n3\n4\n5\n6\n7\n8", testLogoColors)
	if n := strings.Count(strings.TrimRight(text, "\n"), "\n") + 1; n != HeaderHeight {
		t.Errorf("lines = %d, want %d", n, HeaderHeight)
	}
	if strings.Contains(text, "7") {
		t.Errorf("line 7 should be cut: %q", text)
	}
}

func TestProjectLogoColors(t *testing.T) {
	dark := projectLogoColors("#ff8800", false)
	if dark.dot != "#ff8800" || dark.shade == dark.dot || dark.border == dark.shade {
		t.Errorf("dark = %+v, want dot as given and two distinct darker shades", dark)
	}
	if dark.border >= dark.shade {
		t.Errorf("dark border %s should be darker than shade %s", dark.border, dark.shade)
	}
	light := projectLogoColors("#ff8800", true)
	if light.border <= light.shade {
		t.Errorf("light border %s should be lighter than shade %s", light.border, light.shade)
	}
}
