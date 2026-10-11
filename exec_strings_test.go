package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boolean-maybe/tiki/config"
)

// ruki used to skip escape decoding in any string containing a real line break,
// so multi-line descriptions were stored with `\"` instead of `"`.
func TestRunExec_MultiLineDescriptionDecodesEscapes(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	config.ResetPathManager()
	t.Cleanup(config.ResetPathManager)

	steps := []struct {
		stmt string
		want string
	}{
		{`create title="Multi" description="say \"hi\".` + "\n" + `second line"`, "say \"hi\".\nsecond line"},
		{`update where title = "Multi" set description="now \"bye\".` + "\n" + `third line"`, "now \"bye\".\nthird line"},
	}
	for _, s := range steps {
		if got := runExec([]string{s.stmt}); got != exitOK {
			t.Fatalf("exec %q: exit %d, want %d", s.stmt, got, exitOK)
		}
		files, err := filepath.Glob("*.md")
		if err != nil || len(files) != 1 {
			t.Fatalf("exec %q: want one .md file, got %v (err %v)", s.stmt, files, err)
		}
		data, err := os.ReadFile(files[0])
		if err != nil {
			t.Fatalf("read %s: %v", files[0], err)
		}
		if !strings.Contains(string(data), s.want) {
			t.Errorf("exec %q: file does not contain %q:\n%s", s.stmt, s.want, data)
		}
	}
}
