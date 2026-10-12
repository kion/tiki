package controller

import "github.com/boolean-maybe/tiki/model"

type LiveStatusClearer interface {
	Clear(id string) (bool, error)
}

func (pc *PluginController) SetLiveStatus(live LiveStatusClearer) {
	pc.liveStatus = live
}

func (pc *PluginController) handleClearLiveStatus() bool {
	id := pc.getSelectedTikiID(pc.GetFilteredTikisForLane)
	if id == "" {
		return false
	}
	msg, level := "live status is not available", model.MessageLevelError
	if pc.liveStatus != nil {
		cleared, err := pc.liveStatus.Clear(id)
		switch {
		case err != nil:
			msg = "clear live status: " + err.Error()
		case !cleared:
			msg, level = id+" has no live status", model.MessageLevelInfo
		default:
			msg, level = "cleared the live status of "+id, model.MessageLevelInfo
		}
	}
	if pc.statusline != nil {
		pc.statusline.SetMessage(msg, level, true)
	}
	return true
}
