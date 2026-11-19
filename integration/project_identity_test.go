package integration

import (
	"testing"

	"github.com/boolean-maybe/tiki/model"
	"github.com/boolean-maybe/tiki/testutil"
	"github.com/spf13/viper"
)

func TestHeader_ShowsProjectNameBeforeView(t *testing.T) {
	viper.Set("project.name", "Acme")
	t.Cleanup(func() { viper.Set("project.name", "") })
	ta := testutil.NewTestApp(t)
	defer ta.Cleanup()
	if err := ta.LoadPlugins(); err != nil {
		t.Fatalf("LoadPlugins: %v", err)
	}

	ta.NavController.PushView(model.MakePluginViewID("Kanban"), nil)
	ta.Draw()

	if found, _, y := ta.FindText("Acme • Kanban"); !found || y != 0 {
		t.Errorf("header should start with 'Acme • Kanban' on the first row (found=%v, row=%d)", found, y)
	}
}
