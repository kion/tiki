package bootstrap

import (
	"log/slog"

	"github.com/boolean-maybe/tiki/config"
	"github.com/boolean-maybe/tiki/model"
	"github.com/boolean-maybe/tiki/store/tikistore"
)

// InitHeaderAndLayoutModels creates the header config and layout model with
// persisted visibility preferences applied.
func InitHeaderAndLayoutModels() (*model.HeaderConfig, *model.LayoutModel) {
	headerConfig := model.NewHeaderConfig()
	layoutModel := model.NewLayoutModel()

	// Load user preference from saved config
	headerVisible := config.GetHeaderVisible()
	headerConfig.SetUserPreference(headerVisible)
	headerConfig.SetVisible(headerVisible)

	return headerConfig, layoutModel
}

// InitDetailBodyMode restores the detail body mode the last run ended in
// and saves every later change.
func InitDetailBodyMode() *model.DetailBodyMode {
	source := config.LoadUIState().Detail.BodyMode == config.BodyModeSource
	return model.NewDetailBodyMode(source, func(source bool) {
		// read-modify-write, so other remembered keys survive
		st := config.LoadUIState()
		st.Detail.BodyMode = config.BodyModeRendered
		if source {
			st.Detail.BodyMode = config.BodyModeSource
		}
		if err := config.SaveUIState(st); err != nil {
			slog.Warn("could not remember the detail body mode", "error", err)
		}
	})
}

// InitStatuslineModel creates and populates the statusline config with base stats (version, workflow scope, branch, user).
func InitStatuslineModel(tikiStore *tikistore.TikiStore, workflowScope config.Scope) *model.StatuslineConfig {
	cfg := model.NewStatuslineConfig()
	cfg.SetLeftStat("Version", config.Version, 0)
	cfg.SetLeftStat("Workflow", config.WorkflowScopeLabel(workflowScope), 1)
	for _, stat := range tikiStore.GetStats() {
		value := stat.Value
		if stat.Name == "Branch" {
			value = "\ue0a0 " + value
		}
		cfg.SetLeftStat(stat.Name, value, stat.Order)
	}
	return cfg
}
