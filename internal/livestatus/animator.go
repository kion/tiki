package livestatus

import (
	"sync"
	"sync/atomic"
	"time"
)

// Timer is *time.Timer, abstracted so tests can fire it by hand.
type Timer interface {
	Reset(d time.Duration) bool
	Stop() bool
}

func AfterFunc(d time.Duration, f func()) Timer { return time.AfterFunc(d, f) }

// Animator arms one timer for the earliest change any painted status reports,
// rather than ticking at a fixed rate: a static status changes once a minute.
// A redraw that paints no status arms nothing,
// so an idle board, or a view without cards, isn't redrawn at all.
type Animator struct {
	redraw    func(fn func())
	afterFunc func(time.Duration, func()) Timer
	now       func() time.Time

	mu    sync.Mutex
	timer Timer
	due   time.Time // zero while disarmed

	// a redraw still waiting in the UI queue covers this firing too,
	// so a busy UI isn't handed a backlog of identical redraws
	inflight atomic.Bool
}

// redraw is called off the UI goroutine and may block;
// it must run fn on the UI goroutine before drawing.
func NewAnimator(redraw func(fn func()), afterFunc func(time.Duration, func()) Timer, now func() time.Time) *Animator {
	return &Animator{redraw: redraw, afterFunc: afterFunc, now: now}
}

func (a *Animator) Drawn(next time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	due := a.now().Add(next)
	if !a.due.IsZero() && !due.Before(a.due) {
		return
	}
	a.due = due
	if a.timer == nil {
		a.timer = a.afterFunc(next, a.fire)
		return
	}
	a.timer.Reset(next)
}

func (a *Animator) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.timer != nil {
		a.timer.Stop()
		a.timer = nil
	}
	a.due = time.Time{}
}

func (a *Animator) fire() {
	a.mu.Lock()
	// a firing already under way when Stop ran: disarmed, or since re-armed for later
	if a.due.IsZero() || a.now().Before(a.due) {
		a.mu.Unlock()
		return
	}
	a.due = time.Time{}
	a.mu.Unlock()
	if a.inflight.CompareAndSwap(false, true) {
		a.redraw(func() { a.inflight.Store(false) })
	}
}
