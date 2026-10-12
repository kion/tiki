package bootstrap

import (
	"errors"
	"strings"
	"testing"

	"github.com/boolean-maybe/tiki/internal/livestatus"
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

func TestLiveReloadOptions_LiveStatusFollowsLiveReload(t *testing.T) {
	root := t.TempDir()
	if err := livestatus.Write(root, "AAAAAA", livestatus.Status{Message: "Researching"}); err != nil {
		t.Fatal(err)
	}
	reg := livestatus.NewRegistry(livestatus.RegistryOptions{Root: root})
	sl := model.NewStatuslineConfig()
	opts := LiveReloadOptions(root, nil, sl, nil, reg)

	opts.OnStateChange(true)
	if _, ok := reg.Line("AAAAAA"); !ok {
		t.Fatal("status hidden after live reload turned on")
	}
	if _, ok := sl.GetLeftStats()[liveReloadStatKey]; !ok {
		t.Error("live indicator missing: the statusline handler was replaced, not wrapped")
	}

	if err := livestatus.Write(root, "BBBBBB", livestatus.Status{Message: "Testing"}); err != nil {
		t.Fatal(err)
	}
	if opts.Sidecar == nil || opts.Sidecar.Dir != livestatus.Dir(root) {
		t.Fatalf("Sidecar = %+v, want the live status directory", opts.Sidecar)
	}
	opts.Sidecar.OnChange()
	if _, ok := reg.Line("BBBBBB"); !ok {
		t.Fatal("sidecar change not picked up")
	}

	opts.OnStateChange(false)
	if _, ok := reg.Line("AAAAAA"); ok {
		t.Fatal("status shown after live reload turned off")
	}
}
