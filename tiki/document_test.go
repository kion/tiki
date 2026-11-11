package tiki

import (
	"testing"
	"time"

	"github.com/boolean-maybe/ruki"
)

func TestDoc_SatisfiesRukiDocument(t *testing.T) {
	var _ ruki.Document = Doc{}
}

func TestWrapUnwrapDoc_RoundTrip(t *testing.T) {
	tk := New()
	tk.SetID("ABC123")

	if got := UnwrapDoc(WrapDoc(tk)); got != tk {
		t.Fatalf("UnwrapDoc(WrapDoc(t)) = %p, want %p", got, tk)
	}
}

func TestUnwrapDoc_Nil(t *testing.T) {
	if got := UnwrapDoc(nil); got != nil {
		t.Fatalf("UnwrapDoc(nil) = %v, want nil", got)
	}
}

func TestActiveAt_IsTheLaterOfUpdatedAndLive(t *testing.T) {
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		updated time.Time
		live    time.Time
		want    time.Time
	}{
		{"no live status", base, time.Time{}, base},
		{"newer live status", base, base.Add(time.Hour), base.Add(time.Hour)},
		{"older live status", base, base.Add(-time.Hour), base},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tk := New()
			tk.SetUpdatedAt(tt.updated)
			tk.SetLiveAt(tt.live)
			if got := tk.ActiveAt(); !got.Equal(tt.want) {
				t.Errorf("ActiveAt() = %v, want %v", got, tt.want)
			}
			v, ok := WrapDoc(tk).Get("activeAt")
			if at, isTime := v.(time.Time); !ok || !isTime || !at.Equal(tt.want) {
				t.Errorf(`Get("activeAt") = %v, %v; want %v, true`, v, ok, tt.want)
			}
			if !WrapDoc(tk).Has("activeAt") {
				t.Error(`Has("activeAt") = false; a computed field is always present`)
			}
		})
	}
}

func TestClone_KeepsLiveAt(t *testing.T) {
	tk := New()
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	tk.SetLiveAt(at)
	if got := tk.Clone().LiveAt(); !got.Equal(at) {
		t.Errorf("clone LiveAt() = %v, want %v", got, at)
	}
	if got := UnwrapDoc(WrapDoc(tk).Clone()).LiveAt(); !got.Equal(at) {
		t.Errorf("Doc clone LiveAt() = %v, want %v", got, at)
	}
}
