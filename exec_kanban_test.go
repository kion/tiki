package main

import (
	"testing"

	"github.com/boolean-maybe/tiki/config"
)

// The "in-progress before done" rule must only block the transition into done:
// otherwise done tikis can't be edited at all,
// and the after-delete dependsOn cleanup silently skips them.
func TestRunExec_BundledKanbanAllowsEditingDoneTiki(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	config.ResetPathManager()
	t.Cleanup(config.ResetPathManager)

	steps := []struct {
		stmt string
		want int
	}{
		{`create title="Main" assignee="me" status="inProgress"`, exitOK},
		{`update where title = "Main" set status="done"`, exitOK},
		{`update where title = "Main" set tags=tags + ["edited"]`, exitOK},
		{`update where title = "Main" set status="ready"`, exitOK},
		{`update where title = "Main" set status="done"`, exitQueryError},
	}
	for _, s := range steps {
		if got := runExec([]string{s.stmt}); got != s.want {
			t.Fatalf("exec %q: exit %d, want %d", s.stmt, got, s.want)
		}
	}
}
