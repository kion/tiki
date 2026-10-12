package livestatus

import (
	"testing"
	"time"
)

func TestFormatAge(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{in: -5 * time.Second, want: ""},
		{in: 0, want: ""},
		{in: 59*time.Second + 900*time.Millisecond, want: ""},
		{in: time.Minute, want: "1m"},
		{in: 95 * time.Second, want: "1m"},
		{in: 3 * time.Minute, want: "3m"},
		{in: 59*time.Minute + 59*time.Second, want: "59m"},
		{in: time.Hour, want: "1h"},
		{in: 2*time.Hour + 5*time.Minute + 30*time.Second, want: "2h 5m"},
		{in: 24 * time.Hour, want: "1d"},
		{in: 76 * time.Hour, want: "3d 4h"},
	}
	for _, tt := range tests {
		if got := FormatAge(tt.in); got != tt.want {
			t.Errorf("FormatAge(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFrameAt(t *testing.T) {
	cycleStart := time.UnixMilli(720_000) // the cycle is six 120ms frames
	for i, want := range []string{"·", "✢", "✳", "✶", "✻", "✽", "·"} {
		if got := frameAt(cycleStart.Add(time.Duration(i) * 120 * time.Millisecond)); got != want {
			t.Errorf("frame %d = %q, want %q", i, got, want)
		}
	}
	if got := frameAt(cycleStart.Add(119 * time.Millisecond)); got != "·" {
		t.Errorf("mid-frame = %q, want the frame to hold", got)
	}
}

func TestCompose(t *testing.T) {
	base := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC) // a cycle starts here
	st := Status{Agent: "✻ Claude", Message: "Implementing", UpdatedAt: base}

	t.Run("animated", func(t *testing.T) {
		got := Compose(st, base.Add(95*time.Second+225*time.Millisecond), 15*time.Minute)
		want := Line{Agent: "✻ Claude", Glyph: "✢", Age: "1m", Message: "Implementing", Next: 55 * time.Millisecond}
		if got != want {
			t.Fatalf("Compose = %+v, want %+v", got, want)
		}
	})

	t.Run("no age in the first minute", func(t *testing.T) {
		if got := Compose(st, base.Add(59*time.Second), time.Hour).Age; got != "" {
			t.Fatalf("Age = %q, want none", got)
		}
	})

	t.Run("stale freezes on the first frame and wakes for the age", func(t *testing.T) {
		tests := []struct {
			age  time.Duration
			want time.Duration
		}{
			{age: 16*time.Minute + 100*time.Millisecond, want: time.Minute - 100*time.Millisecond},
			{age: 20*time.Minute + 40*time.Second, want: 20 * time.Second},
		}
		for _, tt := range tests {
			got := Compose(st, base.Add(tt.age), 15*time.Minute)
			if !got.Stale || got.Glyph != "·" || got.Next != tt.want {
				t.Errorf("age %v: Compose = %+v, want stale on %q, Next %v", tt.age, got, "·", tt.want)
			}
		}
	})

	t.Run("stale from the moment staleAfter is reached", func(t *testing.T) {
		if got := Compose(st, base.Add(15*time.Minute), 15*time.Minute); !got.Stale {
			t.Fatalf("Compose = %+v, want stale", got)
		}
	})

	t.Run("zero staleAfter never goes stale", func(t *testing.T) {
		if got := Compose(st, base.Add(48*time.Hour), 0); got.Stale {
			t.Fatalf("Compose = %+v, want not stale", got)
		}
	})
}
