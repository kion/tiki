// Package livereload filters events off the UI goroutine,
// so that only real external changes ever queue a UI update.
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

type Target interface {
	ReloadPaths(paths []string) (tikistore.ReloadResult, error)
	Reconcile() (tikistore.ReloadResult, error)
	ExternallyChanged(paths []string) []string
	AllPaths() []string
}

// Callbacks run on the UI goroutine.
type Options struct {
	Root   string
	Target Target

	// Dispatch may block until fn has run (tview's QueueUpdate does),
	// so it is never called from the UI goroutine.
	Dispatch func(fn func())
	Redraw   func()
	// Busy holds batches so a refresh can't move focus or the selection
	// while an overlay or input box is open.
	Busy          func() bool
	OnStateChange func(enabled bool)
	OnError       func(err error)

	Sidecar *Sidecar

	Quiet     time.Duration
	MaxDelay  time.Duration
	BusyRetry time.Duration

	// for tests: replaces the fsnotify watcher
	startSource func(root string, known func() []string, out *batcher, side *sidecarWatch) (io.Closer, error)
}

// Sidecar changes skip the store and are never held while Busy:
// they only repaint, so they can't move focus or the selection.
// OnChange runs on the UI goroutine.
// The catch-up when live reload turns on is OnStateChange's job.
type Sidecar struct {
	Dir      string
	OnChange func()
}

// Enabled and SetEnabled are UI-goroutine only; Stop is safe from anywhere.
type Service struct {
	opts Options

	mu      sync.Mutex
	run     *run
	stopped bool
}

// A fresh run per enable
// lets updates queued by an earlier run detect that they are stale.
type run struct {
	batch  *batcher
	side   *batcher
	source io.Closer
	stop   chan struct{}
	kick   chan struct{}

	mu            sync.Mutex
	held          map[string]struct{}
	heldReconcile bool
	retry         *time.Timer
}

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
		opts.startSource = func(root string, known func() []string, out *batcher, side *sidecarWatch) (io.Closer, error) {
			return startDirWatcher(root, known, out, side)
		}
	}
	return &Service{opts: opts}
}

func (s *Service) Enabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.run != nil
}

// Watches are registered before the catch-up Reconcile,
// so no change slips through in between.
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
	var side *sidecarWatch
	if s.opts.Sidecar != nil {
		r.side = newBatcher(s.opts.Quiet, s.opts.MaxDelay)
		side = &sidecarWatch{dir: s.opts.Sidecar.Dir, out: r.side}
	}
	src, err := s.opts.startSource(s.opts.Root, s.opts.Target.AllPaths, r.batch, side)
	if err != nil {
		r.batch.stop()
		if r.side != nil {
			r.side.stop()
		}
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

// Stop never waits for the UI goroutine:
// the dispatcher may be blocked in a synchronous Dispatch,
// so waiting could deadlock.
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

// The pre-filter runs here, off the UI goroutine,
// so batches of tiki's own writes never queue an update.
// Dispatch can block for as long as the UI is suspended (e.g. in $EDITOR);
// the watcher keeps draining fsnotify meanwhile.
func (s *Service) dispatchLoop(r *run) {
	var sideReady <-chan struct{}
	if r.side != nil {
		sideReady = r.side.ready()
	}
	for {
		select {
		case <-r.stop:
			return
		case <-sideReady:
			r.side.take()
			select {
			case <-r.stop:
				return
			default:
			}
			s.opts.Dispatch(func() { s.applySidecarOnUI(r) })
			continue
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

func (s *Service) applyOnUI(r *run) {
	if !s.isCurrent(r) {
		return // queued before live reload was turned off
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
	// an error is a statusline change too
	if (res.Changed || err != nil) && s.opts.Redraw != nil {
		s.opts.Redraw()
	}
}

func (s *Service) applySidecarOnUI(r *run) {
	if !s.isCurrent(r) {
		return
	}
	s.opts.Sidecar.OnChange()
	if s.opts.Redraw != nil {
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
	if r.side != nil {
		r.side.stop()
	}
	r.mu.Lock()
	if r.retry != nil {
		r.retry.Stop()
	}
	r.mu.Unlock()
}
