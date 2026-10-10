package main

import (
	"testing"

	"github.com/boolean-maybe/tiki/config"
)

// TestRunExec_BundledKanbanAllowsEditingDoneTiki runs exec against the bundled
// kanban workflow (installed into an empty user config). Its "in-progress before
// done" rule must block only the transition into done, not later edits of a
// tiki that is already done — otherwise done tikis can't be changed at all and
// the after-delete dependsOn cleanup silently skips them.
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
