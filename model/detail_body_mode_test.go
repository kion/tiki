package model

import (
	"slices"
	"testing"
)

func TestDetailBodyMode_ToggleFlipsAndReports(t *testing.T) {
	var saved []bool
	m := NewDetailBodyMode(false, func(source bool) { saved = append(saved, source) })

	if m.Source() {
		t.Fatal("should start rendered")
	}
	if !m.Toggle() || !m.Source() {
		t.Error("first toggle should switch to source")
	}
	if m.Toggle() || m.Source() {
		t.Error("second toggle should switch back to rendered")
	}
	if want := []bool{true, false}; !slices.Equal(saved, want) {
		t.Errorf("onChange calls = %v, want %v", saved, want)
	}
}

func TestDetailBodyMode_StartsInGivenModeWithoutCallback(t *testing.T) {
	m := NewDetailBodyMode(true, nil)
	if !m.Source() {
		t.Fatal("should start in source")
	}
	m.Toggle()
	if m.Source() {
		t.Error("toggle without a callback should still flip")
	}
}
