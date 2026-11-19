package config

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// themeNamesWanted is every value appearance.theme accepts.
func themeNamesWanted() []string {
	names := []string{"auto"}
	for name := range themeRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func namesIn(text string, re *regexp.Regexp, skip map[string]bool) []string {
	seen := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		if !skip[m[1]] {
			seen[m[1]] = true
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func TestThemeListsInDocsMatchRegistry(t *testing.T) {
	want := strings.Join(themeNamesWanted(), " ")

	docs, err := os.ReadFile("../docs/config.md")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(docs), "  theme: auto")
	end := strings.Index(string(docs), "  gradientThreshold:")
	if start < 0 || end < start {
		t.Fatal("docs/config.md: theme comment block not found")
	}
	if got := strings.Join(namesIn(string(docs)[start:end], regexp.MustCompile(`"([a-z-]+)"`), nil), " "); got != want {
		t.Errorf("docs/config.md theme list = %s\nwant %s", got, want)
	}

	skill, err := os.ReadFile("../ai/skills/tiki/references/customize-config.md")
	if err != nil {
		t.Fatal(err)
	}
	var line string
	for _, l := range strings.Split(string(skill), "\n") {
		if strings.Contains(l, "theme names:") {
			line = l
		}
	}
	if line == "" {
		t.Fatal("customize-config.md: no line with \"theme names:\"")
	}
	if got := strings.Join(namesIn(line, regexp.MustCompile("`([a-z-]+)`"), nil), " "); got != want {
		t.Errorf("customize-config.md theme list = %s\nwant %s", got, want)
	}
}
