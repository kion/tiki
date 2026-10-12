package livestatus

import (
	"fmt"
	"time"
)

// Next is how soon the line changes again, which arms the redraw timer.
type Line struct {
	Agent   string
	Glyph   string
	Age     string
	Message string
	Stale   bool
	Next    time.Duration
}

const ageStep = time.Minute

// FormatAge shows nothing in the first minute:
// the agent is evidently active, and a seconds count would change every second.
func FormatAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return ""
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	case d < 24*time.Hour:
		return twoUnits(int(d/time.Hour), "h", int(d%time.Hour/time.Minute), "m")
	default:
		return twoUnits(int(d/(24*time.Hour)), "d", int(d%(24*time.Hour)/time.Hour), "h")
	}
}

func twoUnits(major int, majorUnit string, minor int, minorUnit string) string {
	if minor == 0 {
		return fmt.Sprintf("%d%s", major, majorUnit)
	}
	return fmt.Sprintf("%d%s %d%s", major, majorUnit, minor, minorUnit)
}

// Every status plays the same text-style glyphs, whatever harness posts it:
// they take the theme's colour and fill one cell,
// where emoji keep their own colours and vary in width between terminals.
var frames = []string{"·", "✢", "✳", "✶", "✻", "✽"}

const frameInterval = 120 * time.Millisecond

// frameAt derives the frame from the wall clock rather than counting redraws,
// so extra redraws (key presses, reloads) don't speed the animation up.
func frameAt(now time.Time) string {
	return frames[now.UnixMilli()/frameInterval.Milliseconds()%int64(len(frames))]
}

func untilNextFrame(now time.Time) time.Duration {
	ms := frameInterval.Milliseconds()
	return time.Duration(ms-now.UnixMilli()%ms) * time.Millisecond
}

func Compose(st Status, now time.Time, staleAfter time.Duration) Line {
	age := now.Sub(st.UpdatedAt)
	line := Line{
		Agent:   st.Agent,
		Glyph:   frameAt(now),
		Age:     FormatAge(age),
		Message: st.Message,
		Next:    untilNextFrame(now),
	}
	if staleAfter > 0 && age >= staleAfter {
		// a frozen status changes only when its age does
		line.Stale, line.Glyph = true, frames[0]
		line.Next = ageStep - age%ageStep
	}
	return line
}
