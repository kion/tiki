package bootstrap

import (
	"testing"

	"github.com/boolean-maybe/tiki/config"
	"github.com/boolean-maybe/tiki/model"
	"github.com/spf13/viper"
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

func TestAddProjectDirStat(t *testing.T) {
	t.Chdir(t.TempDir())
	config.ResetPathManager()
	t.Cleanup(config.ResetPathManager)

	cases := []struct {
		name, color, wantFg, wantBg string
	}{
		{"plain", "", "", ""},
		{"coloured", "#ff8800", "#000000", "#ff8800"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			viper.Set("project.color", tc.color)
			t.Cleanup(func() { viper.Set("project.color", "") })
			cfg := model.NewStatuslineConfig()

			addProjectDirStat(cfg)

			stat, ok := cfg.GetLeftStats()["Dir"]
			if !ok {
				t.Fatal("no Dir stat")
			}
			if stat.Value != config.ProjectDirLabel() || stat.Priority != -1 || stat.Fg != tc.wantFg || stat.Bg != tc.wantBg {
				t.Errorf("Dir = %+v", stat)
			}
		})
	}
}
