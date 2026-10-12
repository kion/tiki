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
	clearLiveStatus(pc.liveStatus, pc.statusline, id)
	return true
}

func (dc *DetailController) SetLiveStatus(live LiveStatusClearer) {
	dc.liveStatus = live
}

func (dc *DetailController) handleClearLiveStatus() bool {
	if dc.selectedTikiID == "" {
		return false
	}
	clearLiveStatus(dc.liveStatus, dc.statusline, dc.selectedTikiID)
	return true
}

func clearLiveStatus(live LiveStatusClearer, statusline *model.StatuslineConfig, id string) {
	msg, level := "live status is not available", model.MessageLevelError
	if live != nil {
		cleared, err := live.Clear(id)
		switch {
		case err != nil:
			msg = "clear live status: " + err.Error()
		case !cleared:
			msg, level = id+" has no live status", model.MessageLevelInfo
		default:
			msg, level = "cleared the live status of "+id, model.MessageLevelInfo
		}
	}
	if statusline != nil {
		statusline.SetMessage(msg, level, true)
	}
}
