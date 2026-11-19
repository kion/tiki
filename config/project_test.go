package config

import (
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func setProjectSetting(t *testing.T, key, value string) {
	t.Helper()
	viper.Set(key, value)
	t.Cleanup(func() { viper.Set(key, "") })
}

func TestGetProjectName_StripsControlCharacters(t *testing.T) {
	setProjectSetting(t, "project.name", "\x1b]0;evil\x07Acme\u009b\n")

	if got := GetProjectName(); got != "]0;evilAcme" {
		t.Errorf("GetProjectName() = %q, want control characters removed", got)
	}
}

func TestGetProjectName_DefaultsToFolderName(t *testing.T) {
	setupResolveFilesTest(t)
	setProjectSetting(t, "project.name", "   ")

	want := filepath.Base(mustGetPathManager().projectRoot)
	if got := GetProjectName(); got != want {
		t.Errorf("GetProjectName() = %q, want folder name %q", got, want)
	}
}

func TestTerminalTitle(t *testing.T) {
	setProjectSetting(t, "project.name", "Acme")

	if got := TerminalTitle(); got != "Acme • tiki" {
		t.Errorf("TerminalTitle() = %q", got)
	}
}

func TestParseProjectColor(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"#ff8800", "#ff8800", true},
		{"#FF8800", "#ff8800", true},
		{"#f80", "#ff8800", true},
		{" orange ", "#ffa500", true},
		{"default", "", false},
		{"reset", "", false},
		{"nope", "", false},
		{"#12345", "", false},
		{"", "", false},
		{"[red]", "", false},
	}
	for _, tc := range cases {
		got, ok := ParseProjectColor(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("ParseProjectColor(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestGetProjectLogo_Sanitised(t *testing.T) {
	setProjectSetting(t, "project.logo", "a\tb\x1b[31m\r\n\tc\u0085\n\n")

	want := "a       b[31m\n        c"
	if got := GetProjectLogo(); got != want {
		t.Errorf("GetProjectLogo() = %q, want %q", got, want)
	}
}

func TestShortenDir(t *testing.T) {
	cases := []struct {
		path, home string
		max        int
		want       string
	}{
		{"/home/u/proj", "/home/u", 40, "~/proj"},
		{"/home/u", "/home/u", 40, "~"},
		{"/home/u2/x", "/home/u", 40, "/home/u2/x"},
		{"/home/u/stor/devel/projects/.config/tiki", "/home/u", 20, "~/s/d/p/.c/tiki"},
		{"/very/long/path/aaaaaaaaaaa/bbbbbbbbbbbbb/project", "/home/u", 20, "/v/l/p/a/b/project"},
		{"/home/u/a/averyveryveryverylongprojectfoldername", "/home/u", 20, "~/a/averyveryveryverylongprojectfoldername"},
		{"/srv/x", "", 40, "/srv/x"},
	}
	for _, tc := range cases {
		if got := shortenDir(tc.path, tc.home, tc.max); got != tc.want {
			t.Errorf("shortenDir(%q, %q, %d) = %q, want %q", tc.path, tc.home, tc.max, got, tc.want)
		}
	}
}

func TestContrastingText(t *testing.T) {
	cases := map[string]string{"#ffff00": "#000000", "#ff8800": "#000000", "#000080": "#ffffff", "#202020": "#ffffff"}
	for bg, want := range cases {
		if got := ContrastingText(bg); got != want {
			t.Errorf("ContrastingText(%s) = %s, want %s", bg, got, want)
		}
	}
}
