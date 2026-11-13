package bootstrap

import (
	"testing"

	"github.com/boolean-maybe/tiki/config"
)

func TestInitDetailBodyMode_RemembersTheLastModeAcrossRuns(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	config.ResetPathManager()
	t.Cleanup(config.ResetPathManager)

	firstRun := InitDetailBodyMode()
	if firstRun.Source() {
		t.Fatal("with no saved state the body should start rendered")
	}
	firstRun.Toggle()

	secondRun := InitDetailBodyMode()
	if !secondRun.Source() {
		t.Fatal("the next run should start in source")
	}
	secondRun.Toggle()

	if InitDetailBodyMode().Source() {
		t.Error("switching back to rendered should be remembered too")
	}
}
