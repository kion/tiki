package livereload

import (
	"sync"
	"time"
)

// batcher coalesces changed paths into batches. A batch is due once no new
// change has arrived for the quiet period, or once maxDelay has passed since
// its first change, whichever comes first — so a burst of writes becomes one
// reload, while an agent that never pauses still shows progress.
//
// Safe for concurrent use: the watcher adds, the dispatcher takes.
type batcher struct {
	quiet    time.Duration
	maxDelay time.Duration
	readyC   chan struct{} // capacity 1: "a batch is due"

	mu        sync.Mutex
	paths     map[string]struct{}
	reconcile bool
	first     time.Time // arrival of the oldest pending change
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

// add records changed paths and (re)arms the flush timer.
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

// requestReconcile asks for a full git-free reconcile with the next batch.
func (b *batcher) requestReconcile() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.reconcile = true
	b.armLocked()
}

// take drains the pending batch.
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

// ready signals when a batch is due.
func (b *batcher) ready() <-chan struct{} { return b.readyC }

// stop cancels the flush timer; later adds are kept but never signalled.
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
	default: // already signalled; the consumer takes everything at once
	}
}
