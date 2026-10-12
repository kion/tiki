package model

import "testing"

// The action runs after the dialog hides,
// so focus is restored before a delete that leaves the view can move it.
func TestConfirmConfig_ConfirmHidesThenRuns(t *testing.T) {
	cc := NewConfirmConfig()
	ran := false
	visibleDuringRun := true
	cc.SetOnConfirm(func() {
		ran = true
		visibleDuringRun = cc.IsVisible()
	})
	cc.SetVisible(true)

	cc.Confirm()

	if !ran {
		t.Fatal("Confirm did not run the action")
	}
	if visibleDuringRun {
		t.Error("dialog still visible while the action ran")
	}
	if cc.IsVisible() {
		t.Error("dialog visible after Confirm")
	}
}

func TestConfirmConfig_CancelHidesWithoutRunning(t *testing.T) {
	cc := NewConfirmConfig()
	ran := false
	cc.SetOnConfirm(func() { ran = true })
	cc.SetVisible(true)

	cc.Cancel()

	if ran {
		t.Error("Cancel ran the action")
	}
	if cc.IsVisible() {
		t.Error("dialog visible after Cancel")
	}
}

func TestConfirmConfig_ListenerSeesVisibilityChanges(t *testing.T) {
	cc := NewConfirmConfig()
	var seen []bool
	cc.AddListener(func() { seen = append(seen, cc.IsVisible()) })

	cc.SetVisible(true)
	cc.SetVisible(true)
	cc.Cancel()

	if len(seen) != 2 || !seen[0] || seen[1] {
		t.Errorf("listener saw %v, want [true false]", seen)
	}
}
