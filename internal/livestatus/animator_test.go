package livestatus

import (
	"sync"
	"testing"
	"time"
)

type fakeTimer struct {
	mu      sync.Mutex
	fire    func()
	armed   []time.Duration
	stopped bool
}

func (f *fakeTimer) Reset(d time.Duration) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.armed = append(f.armed, d)
	return true
}

func (f *fakeTimer) Stop() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = true
	return true
}

func (f *fakeTimer) last() time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.armed[len(f.armed)-1]
}

func (f *fakeTimer) arms() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.armed)
}

type animatorHarness struct {
	anim    *Animator
	now     time.Time
	timers  []*fakeTimer
	redraws []func()
}

func newAnimatorHarness() *animatorHarness {
	h := &animatorHarness{now: time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)}
	h.anim = NewAnimator(
		func(fn func()) { h.redraws = append(h.redraws, fn) },
		func(d time.Duration, fire func()) Timer {
			ft := &fakeTimer{fire: fire, armed: []time.Duration{d}}
			h.timers = append(h.timers, ft)
			return ft
		},
		func() time.Time { return h.now },
	)
	return h
}

func (h *animatorHarness) timer(t *testing.T) *fakeTimer {
	t.Helper()
	if len(h.timers) == 0 {
		t.Fatal("no timer armed")
	}
	return h.timers[len(h.timers)-1]
}

// fire runs the timer as the runtime would;
// the tests that fire it arm it for a second.
func (h *animatorHarness) fire(t *testing.T) {
	t.Helper()
	h.now = h.now.Add(time.Second)
	h.timer(t).fire()
}

func (h *animatorHarness) takeRedraw(t *testing.T) func() {
	t.Helper()
	if len(h.redraws) != 1 {
		t.Fatalf("redraws queued = %d, want 1", len(h.redraws))
	}
	fn := h.redraws[0]
	h.redraws = nil
	return fn
}

func TestAnimator_DrawArmsForTheNextChange(t *testing.T) {
	h := newAnimatorHarness()
	h.anim.Drawn(120 * time.Millisecond)
	if got := h.timer(t).last(); got != 120*time.Millisecond {
		t.Fatalf("armed for %v, want 120ms", got)
	}
}

func TestAnimator_TheEarliestChangeWins(t *testing.T) {
	h := newAnimatorHarness()
	h.anim.Drawn(time.Minute)
	h.anim.Drawn(30 * time.Second)
	h.anim.Drawn(45 * time.Second)
	ft := h.timer(t)
	if got := ft.last(); got != 30*time.Second {
		t.Fatalf("armed for %v, want 30s", got)
	}

	// a redraw on a key press, 20s on, reports the same change from closer
	h.now = h.now.Add(20 * time.Second)
	h.anim.Drawn(10 * time.Second)
	h.anim.Drawn(40 * time.Second)
	if got := ft.arms(); got != 2 {
		t.Fatalf("timer armed %d times, want 2: one change shouldn't re-arm", got)
	}
}

func TestAnimator_FiringRedrawsOnce(t *testing.T) {
	h := newAnimatorHarness()
	h.anim.Drawn(time.Second)
	h.fire(t)
	h.takeRedraw(t)()
}

func TestAnimator_RedrawWithoutStatusesDoesNotRearm(t *testing.T) {
	h := newAnimatorHarness()
	h.anim.Drawn(time.Second)
	ft := h.timer(t)
	h.fire(t)
	h.takeRedraw(t)() // nothing drawn

	if got := ft.arms(); got != 1 {
		t.Fatalf("timer armed %d times, want once", got)
	}
}

func TestAnimator_RearmsAfterFiring(t *testing.T) {
	h := newAnimatorHarness()
	h.anim.Drawn(time.Second)
	ft := h.timer(t)
	h.fire(t)
	h.takeRedraw(t)()

	h.anim.Drawn(time.Minute)
	if ft.arms() != 2 || ft.last() != time.Minute {
		t.Fatalf("armed %v, want a second arm for 1m", ft.armed)
	}
}

func TestAnimator_CoalescesRedrawsTheUIHasNotRunYet(t *testing.T) {
	h := newAnimatorHarness()
	h.anim.Drawn(time.Second)
	h.fire(t)
	pending := h.takeRedraw(t)

	// a draw of something else, before the queued redraw
	h.anim.Drawn(time.Second)
	h.fire(t)
	if len(h.redraws) != 0 {
		t.Fatal("queued a second redraw behind one the UI hasn't run")
	}

	pending()
	h.anim.Drawn(time.Second)
	h.fire(t)
	h.takeRedraw(t)
}

func TestAnimator_StopDisarms(t *testing.T) {
	h := newAnimatorHarness()
	h.anim.Drawn(time.Second)
	ft := h.timer(t)
	h.anim.Stop()
	if !ft.stopped {
		t.Fatal("timer not stopped")
	}

	// a firing that was already under way
	h.fire(t)
	if len(h.redraws) != 0 {
		t.Fatal("redrew after Stop")
	}

	h.anim.Drawn(time.Second)
	if len(h.timers) != 2 {
		t.Fatal("a draw after Stop didn't arm again")
	}
}

func TestAnimator_RealTimerFires(t *testing.T) {
	redraws := make(chan func(), 1)
	anim := NewAnimator(func(fn func()) { redraws <- fn }, AfterFunc, time.Now)
	defer anim.Stop()
	anim.Drawn(10 * time.Millisecond)
	select {
	case fn := <-redraws:
		fn()
	case <-time.After(time.Second):
		t.Fatal("no redraw")
	}
}
