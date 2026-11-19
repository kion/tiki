package config

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/boolean-maybe/tiki/theme"
	"github.com/mattn/go-runewidth"
	"github.com/spf13/viper"
)

// Project values may come from a cloned repo's tiki-config.yaml,
// so every getter strips control characters before the text reaches the terminal.

const (
	titleSeparator = " • "
	logoTabWidth   = 8
	dirLabelMax    = 40
)

// GetProjectName returns project.name, or the name of the folder tiki runs in.
func GetProjectName() string {
	if name := sanitizeLine(strings.TrimSpace(viper.GetString("project.name"))); name != "" {
		return name
	}
	return sanitizeLine(filepath.Base(mustGetPathManager().projectRoot))
}

// TerminalTitle is the title tiki gives its terminal tab or window.
func TerminalTitle() string {
	return GetProjectName() + titleSeparator + "tiki"
}

// GetProjectColor returns project.color as "#rrggbb", or false when unset or invalid.
func GetProjectColor() (string, bool) {
	return ParseProjectColor(viper.GetString("project.color"))
}

// ParseProjectColor normalises a hex colour (#rrggbb or #rgb) or a colour name tcell knows to "#rrggbb".
// Only this normalised form ever reaches a tview colour tag.
func ParseProjectColor(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) == 4 && s[0] == '#' {
		s = string([]byte{'#', s[1], s[1], s[2], s[2], s[3], s[3]})
	}
	if s == "" || strings.ContainsAny(s, "[]") {
		return "", false
	}
	c := theme.NewColorHex(s)
	// unknown names, "default" and "reset" don't name a real colour
	if r, g, b := c.RGB(); c.IsDefault() || r < 0 || g < 0 || b < 0 {
		return "", false
	}
	return c.Hex(), true
}

// GetProjectLogo returns project.logo with tabs expanded, control characters removed
// and trailing blank lines trimmed; "" when unset.
func GetProjectLogo() string {
	return sanitizeBlock(viper.GetString("project.logo"))
}

// ProjectDirLabel is the project folder for the bottom bar: home shortened to ~,
// and middle folders to their first letter when it's long.
func ProjectDirLabel() string {
	home, _ := os.UserHomeDir()
	return shortenDir(mustGetPathManager().projectRoot, home, dirLabelMax)
}

func sanitizeLine(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

func sanitizeBlock(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = sanitizeLine(expandTabs(line))
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n ")
}

func expandTabs(line string) string {
	if !strings.ContainsRune(line, '\t') {
		return line
	}
	var b strings.Builder
	col := 0
	for _, r := range line {
		if r == '\t' {
			n := logoTabWidth - col%logoTabWidth
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		}
		b.WriteRune(r)
		col += runewidth.RuneWidth(r)
	}
	return b.String()
}

// shortenDir shows path with home as ~ (filepath.Rel, so /home/u2 isn't mistaken for /home/u),
// and when it's wider than max, shortens every middle folder to its first letter.
// The last folder is never shortened: it's the one that names the project.
func shortenDir(path, home string, max int) string {
	display := path
	if home != "" {
		if rel, err := filepath.Rel(home, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			display = "~"
			if rel != "." {
				display += string(filepath.Separator) + rel
			}
		}
	}
	if runewidth.StringWidth(display) <= max {
		return display
	}
	sep := string(filepath.Separator)
	parts := strings.Split(display, sep)
	for i := 1; i < len(parts)-1; i++ {
		parts[i] = shortFolder(parts[i])
	}
	return strings.Join(parts, sep)
}

func shortFolder(name string) string {
	if name == "" {
		return name
	}
	r, size := utf8.DecodeRuneInString(name)
	if r == '.' && size < len(name) {
		next, _ := utf8.DecodeRuneInString(name[size:])
		return "." + string(next)
	}
	return string(r)
}

// ContrastingText returns black or white, whichever reads better on the "#rrggbb" background.
func ContrastingText(bgHex string) string {
	r, g, b := theme.NewColorHex(bgHex).RGB()
	// perceived luminance (ITU-R BT.601)
	if 299*r+587*g+114*b >= 128000 {
		return "#000000"
	}
	return "#ffffff"
}
