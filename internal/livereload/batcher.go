package livereload

import (
	"sync"
	"time"
)

// The max delay keeps an agent that never pauses from starving the quiet period;
// a burst of writes still becomes one reload.
type batcher struct {
	quiet    time.Duration
	maxDelay time.Duration
	readyC   chan struct{}

	mu        sync.Mutex
	paths     map[string]struct{}
	reconcile bool
	first     time.Time
	timer     *time.Timer
	stopped   bool
}

func newBatcher(quiet, maxDelay time.Duration) *batcher {
	return &batcher{
		quiet:    quiet,
		maxDelay: maxDelay,
		readyC:   make(chan struct{}, 1),
		paths:    map[string]struct{}{},
	}
}

func (b *batcher) add(paths ...string) {
	if len(paths) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, p := range paths {
		b.paths[p] = struct{}{}
	}
	b.armLocked()
}

func (b *batcher) requestReconcile() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.reconcile = true
	b.armLocked()
}

func (b *batcher) take() (paths []string, reconcile bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	paths = make([]string, 0, len(b.paths))
	for p := range b.paths {
		paths = append(paths, p)
	}
	reconcile = b.reconcile
	b.paths = map[string]struct{}{}
	b.reconcile = false
	b.first = time.Time{}
	return paths, reconcile
}

func (b *batcher) ready() <-chan struct{} { return b.readyC }

func (b *batcher) stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stopped = true
	if b.timer != nil {
		b.timer.Stop()
	}
}

func (b *batcher) armLocked() {
	if b.stopped {
		return
	}
	now := time.Now()
	if b.first.IsZero() {
		b.first = now
	}
	due := now.Add(b.quiet)
	if limit := b.first.Add(b.maxDelay); limit.Before(due) {
		due = limit
	}
	wait := due.Sub(now)
	if b.timer == nil {
		b.timer = time.AfterFunc(wait, b.fire)
		return
	}
	b.timer.Reset(wait)
}

func (b *batcher) fire() {
	b.mu.Lock()
	pending := len(b.paths) > 0 || b.reconcile
	b.mu.Unlock()
	if !pending {
		return
	}
	select {
	case b.readyC <- struct{}{}:
	default: // the consumer takes everything at once
	}
}
