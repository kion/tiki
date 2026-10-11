package bootstrap

import (
	"errors"
	"strings"
	"testing"

	"github.com/boolean-maybe/tiki/model"
)

func TestLiveReloadStatus_IndicatorFollowsState(t *testing.T) {
	sl := model.NewStatuslineConfig()
	onState, _ := liveReloadStatus(sl)

	onState(true)
	stat, ok := sl.GetLeftStats()[liveReloadStatKey]
	if !ok || !strings.Contains(stat.Value, "live") {
		t.Fatalf("enabled: left stats = %v, want a live indicator", sl.GetLeftStats())
	}

	onState(false)
	if _, ok := sl.GetLeftStats()[liveReloadStatKey]; ok {
		t.Error("disabled: live indicator still shown")
	}
}

func TestLiveReloadStatus_ErrorsReachStatusline(t *testing.T) {
	sl := model.NewStatuslineConfig()
	_, onErr := liveReloadStatus(sl)

	onErr(errors.New("live reload unavailable: too many open files"))

	msg, level, _ := sl.GetMessage()
	if !strings.Contains(msg, "too many open files") {
		t.Errorf("message = %q, want the error text", msg)
	}
	if level != model.MessageLevelError {
		t.Errorf("level = %v, want error", level)
	}
}
