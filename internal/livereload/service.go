// Package livereload watches the document root and applies external changes
// to the store while the TUI runs, so edits made by other processes (AI
// agents running `tiki exec`, editors, git) show up without pressing Refresh.
//
// Pipeline: fsnotify → batcher (debounce) → background pre-filter that drops
// tiki's own writes (Target.ExternallyChanged) → one queued update on the UI
// goroutine → Target.ReloadPaths / Target.Reconcile → store listeners
// re-render. Only batches with real external changes reach the UI goroutine.
package livereload

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/boolean-maybe/tiki/store/tikistore"
)

const (
	defaultQuiet     = 200 * time.Millisecond
	defaultMaxDelay  = time.Second
	defaultBusyRetry = 300 * time.Millisecond
)

// Target is the store side of live reload. *tikistore.TikiStore satisfies it.
type Target interface {
	ReloadPaths(paths []string) (tikistore.ReloadResult, error)
	Reconcile() (tikistore.ReloadResult, error)
	ExternallyChanged(paths []string) []string
	AllPaths() []string
}

// Options configures a Service. Callbacks marked "UI goroutine" are only
// ever invoked from functions run through Dispatch or from SetEnabled.
type Options struct {
	// Root is the document root to watch (the store's directory).
	Root   string
	Target Target

	// Dispatch runs fn on the UI goroutine. It may block until fn has run
	// (tview's QueueUpdate does). It is never called from the UI goroutine.
	Dispatch func(fn func())
	// Redraw repaints the screen after a batch changed the store. UI
	// goroutine. Optional.
	Redraw func()
	// Busy reports that the user is mid-interaction (an overlay or input box
	// is open); batches are held until it clears so a refresh cannot move
	// focus or the selection under them. UI goroutine. Optional.
	Busy func() bool
	// OnStateChange reports live reload turning on or off. UI goroutine.
	// Optional.
	OnStateChange func(enabled bool)
	// OnError reports a failure to start watching or to apply a batch. UI
	// goroutine. Optional.
	OnError func(err error)

	// Quiet, MaxDelay and BusyRetry tune batching; zero means the default
	// (200ms quiet period, 1s max latency, 300ms busy re-check).
	Quiet     time.Duration
	MaxDelay  time.Duration
	BusyRetry time.Duration

	// startSource replaces the fsnotify watcher in tests.
	startSource func(root string, known func() []string, out *batcher) (io.Closer, error)
}

// Service turns live reload on and off. Enabled and SetEnabled belong to the
// UI goroutine; Stop may be called from anywhere.
type Service struct {
	opts Options

	mu      sync.Mutex
	run     *run // nil while disabled
	stopped bool
}

// run is one enabled period. A new run starts on every enable, so updates
// queued by a previous run recognise themselves as stale.
type run struct {
	batch  *batcher
	source io.Closer
	stop   chan struct{}
	kick   chan struct{} // capacity 1: re-offer held changes

	mu            sync.Mutex
	held          map[string]struct{}
	heldReconcile bool
	retry         *time.Timer
}

// New returns a disabled Service.
func New(opts Options) *Service {
	if opts.Quiet <= 0 {
		opts.Quiet = defaultQuiet
	}
	if opts.MaxDelay <= 0 {
		opts.MaxDelay = defaultMaxDelay
	}
	if opts.BusyRetry <= 0 {
		opts.BusyRetry = defaultBusyRetry
	}
	if opts.startSource == nil {
		opts.startSource = func(root string, known func() []string, out *batcher) (io.Closer, error) {
			return startDirWatcher(root, known, out)
		}
	}
	return &Service{opts: opts}
}

// Enabled reports whether live reload is on.
func (s *Service) Enabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.run != nil
}

// SetEnabled turns live reload on or off. Call it on the UI goroutine (or
// before the UI starts). Turning it on registers the watches first and then
// reconciles the store with disk, so no change made while live reload was off
// — or while the watches were being set up — is missed.
func (s *Service) SetEnabled(on bool) error {
	if on == s.Enabled() {
		return nil
	}
	if !on {
		s.stopRun()
		s.stateChanged(false)
		return nil
	}

	r := &run{
		batch: newBatcher(s.opts.Quiet, s.opts.MaxDelay),
		stop:  make(chan struct{}),
		kick:  make(chan struct{}, 1),
		held:  map[string]struct{}{},
	}
	src, err := s.opts.startSource(s.opts.Root, s.opts.Target.AllPaths, r.batch)
	if err != nil {
		r.batch.stop()
		err = fmt.Errorf("live reload unavailable: %w", err)
		s.reportError(err)
		return err
	}
	r.source = src

	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		r.shutdown()
		return nil
	}
	s.run = r
	s.mu.Unlock()

	go s.dispatchLoop(r)
	s.stateChanged(true)
	if _, err := s.opts.Target.Reconcile(); err != nil {
		s.reportError(fmt.Errorf("live reload: %w", err))
	}
	return nil
}

// Stop turns live reload off for good (application shutdown). It never waits
// for the UI goroutine, so it is safe from any goroutine. Idempotent.
func (s *Service) Stop() {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
	s.stopRun()
}

func (s *Service) stopRun() {
	s.mu.Lock()
	r := s.run
	s.run = nil
	s.mu.Unlock()
	if r != nil {
		r.shutdown()
	}
}

func (s *Service) isCurrent(r *run) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.run == r
}

// dispatchLoop moves due batches toward the UI goroutine. It runs the
// pre-filter here, off the UI goroutine, so batches made only of tiki's own
// writes are dropped without queuing anything. Dispatch may block for as
// long as the UI is busy (or suspended in an editor); meanwhile the watcher
// keeps draining fsnotify into the batcher.
func (s *Service) dispatchLoop(r *run) {
	for {
		select {
		case <-r.stop:
			return
		case <-r.batch.ready():
			paths, reconcile := r.batch.take()
			if len(paths) > 0 {
				paths = s.opts.Target.ExternallyChanged(paths)
			}
			r.hold(paths, reconcile)
		case <-r.kick:
		}
		if !r.hasHeld() {
			continue
		}
		select {
		case <-r.stop:
			return
		default:
		}
		s.opts.Dispatch(func() { s.applyOnUI(r) })
	}
}

// applyOnUI applies the held changes. Runs on the UI goroutine.
func (s *Service) applyOnUI(r *run) {
	if !s.isCurrent(r) {
		return // queued by a run that has since been turned off
	}
	if s.opts.Busy != nil && s.opts.Busy() {
		r.recheckAfter(s.opts.BusyRetry)
		return
	}
	paths, reconcile := r.takeHeld()
	if len(paths) == 0 && !reconcile {
		return
	}

	var res tikistore.ReloadResult
	var err error
	if reconcile {
		res, err = s.opts.Target.Reconcile() // covers the held paths too
	} else {
		res, err = s.opts.Target.ReloadPaths(paths)
	}
	if err != nil {
		s.reportError(fmt.Errorf("live reload: %w", err))
	}
	if len(res.Retry) > 0 {
		r.batch.add(res.Retry...)
	}
	if res.Changed && s.opts.Redraw != nil {
		s.opts.Redraw()
	}
}

func (s *Service) stateChanged(enabled bool) {
	if s.opts.OnStateChange != nil {
		s.opts.OnStateChange(enabled)
	}
}

func (s *Service) reportError(err error) {
	if s.opts.OnError != nil {
		s.opts.OnError(err)
	}
}

func (r *run) hold(paths []string, reconcile bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range paths {
		r.held[p] = struct{}{}
	}
	r.heldReconcile = r.heldReconcile || reconcile
}

func (r *run) hasHeld() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.held) > 0 || r.heldReconcile
}

func (r *run) takeHeld() ([]string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	paths := make([]string, 0, len(r.held))
	for p := range r.held {
		paths = append(paths, p)
	}
	reconcile := r.heldReconcile
	r.held = map[string]struct{}{}
	r.heldReconcile = false
	return paths, reconcile
}

// recheckAfter re-offers the held changes after d.
func (r *run) recheckAfter(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.retry != nil {
		r.retry.Reset(d)
		return
	}
	r.retry = time.AfterFunc(d, func() {
		select {
		case r.kick <- struct{}{}:
		default:
		}
	})
}

func (r *run) shutdown() {
	close(r.stop)
	if r.source != nil {
		_ = r.source.Close()
	}
	r.batch.stop()
	r.mu.Lock()
	if r.retry != nil {
		r.retry.Stop()
	}
	r.mu.Unlock()
}
