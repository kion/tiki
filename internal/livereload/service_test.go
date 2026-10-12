package livereload

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/boolean-maybe/tiki/store/tikistore"
)

type fakeTarget struct {
	mu         sync.Mutex
	log        []string
	reloads    [][]string
	reloaded   chan []string
	reconciled chan struct{}
	checked    chan []string
	external   func(string) bool
	result     func(paths []string) tikistore.ReloadResult
	reloadErr  error
}

func newFakeTarget() *fakeTarget {
	return &fakeTarget{
		reloaded:   make(chan []string, 64),
		reconciled: make(chan struct{}, 64),
		checked:    make(chan []string, 64),
	}
}

func (f *fakeTarget) record(entry string) {
	f.mu.Lock()
	f.log = append(f.log, entry)
	f.mu.Unlock()
}

func (f *fakeTarget) ReloadPaths(paths []string) (tikistore.ReloadResult, error) {
	f.record("reload")
	f.mu.Lock()
	f.reloads = append(f.reloads, paths)
	f.mu.Unlock()
	res := tikistore.ReloadResult{}
	if f.result != nil {
		res = f.result(paths)
	}
	f.reloaded <- paths
	return res, f.reloadErr
}

func (f *fakeTarget) Reconcile() (tikistore.ReloadResult, error) {
	f.record("reconcile")
	f.reconciled <- struct{}{}
	return tikistore.ReloadResult{}, nil
}

func (f *fakeTarget) ExternallyChanged(paths []string) []string {
	select {
	case f.checked <- paths:
	default:
	}
	if f.external == nil {
		return paths
	}
	var out []string
	for _, p := range paths {
		if f.external(p) {
			out = append(out, p)
		}
	}
	return out
}

func (f *fakeTarget) AllPaths() []string { return nil }

func (f *fakeTarget) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.log...)
}

type fakeSource struct {
	mu     sync.Mutex
	batch  *batcher
	side   *batcher
	closed bool
	err    error
	target *fakeTarget
}

func (fs *fakeSource) start(_ string, _ func() []string, out *batcher, side *sidecarWatch) (io.Closer, error) {
	if fs.err != nil {
		return nil, fs.err
	}
	if fs.target != nil {
		fs.target.record("start")
	}
	fs.mu.Lock()
	fs.batch = out
	if side != nil {
		fs.side = side.out
	}
	fs.mu.Unlock()
	return fs, nil
}

func (fs *fakeSource) pushSide(paths ...string) {
	fs.mu.Lock()
	b := fs.side
	fs.mu.Unlock()
	b.add(paths...)
}

func (fs *fakeSource) Close() error {
	fs.mu.Lock()
	fs.closed = true
	fs.mu.Unlock()
	return nil
}

func (fs *fakeSource) push(paths ...string) {
	fs.mu.Lock()
	b := fs.batch
	fs.mu.Unlock()
	b.add(paths...)
}

type harness struct {
	svc      *Service
	target   *fakeTarget
	source   *fakeSource
	states   chan bool
	errs     chan error
	redraws  atomic.Int32
	redrawn  chan struct{}
	dispatch atomic.Int32
	busy     atomic.Bool
}

func newHarness(t *testing.T, mutate func(*Options)) *harness {
	t.Helper()
	h := &harness{
		target:  newFakeTarget(),
		states:  make(chan bool, 16),
		errs:    make(chan error, 16),
		redrawn: make(chan struct{}, 16),
	}
	h.source = &fakeSource{target: h.target}
	opts := Options{
		Root:   t.TempDir(),
		Target: h.target,
		Dispatch: func(fn func()) {
			h.dispatch.Add(1)
			fn()
		},
		Redraw: func() {
			h.redraws.Add(1)
			select {
			case h.redrawn <- struct{}{}:
			default:
			}
		},
		Busy:          h.busy.Load,
		OnStateChange: func(on bool) { h.states <- on },
		OnError:       func(err error) { h.errs <- err },
		Quiet:         10 * time.Millisecond,
		MaxDelay:      50 * time.Millisecond,
		BusyRetry:     20 * time.Millisecond,
		startSource:   h.source.start,
	}
	if mutate != nil {
		mutate(&opts)
	}
	h.svc = New(opts)
	t.Cleanup(h.svc.Stop)
	return h
}

func recvPaths(t *testing.T, ch <-chan []string) []string {
	t.Helper()
	select {
	case p := <-ch:
		return p
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a reload")
		return nil
	}
}

func mustEnable(t *testing.T, h *harness) {
	t.Helper()
	if err := h.svc.SetEnabled(true); err != nil {
		t.Fatalf("SetEnabled(true): %v", err)
	}
	<-h.target.reconciled // drain the catch-up reconcile
}

func TestService_EnableWatchesBeforeCatchUpReconcile(t *testing.T) {
	h := newHarness(t, nil)
	mustEnable(t, h)

	calls := h.target.calls()
	if len(calls) != 2 || calls[0] != "start" || calls[1] != "reconcile" {
		t.Errorf("call order = %v, want [start reconcile]", calls)
	}
	if !h.svc.Enabled() {
		t.Error("Enabled() = false after enabling")
	}
	if on := <-h.states; !on {
		t.Error("state change not reported as enabled")
	}
}

func TestService_StartFailureLeavesDisabled(t *testing.T) {
	h := newHarness(t, nil)
	h.source.err = errors.New("too many open files")

	if err := h.svc.SetEnabled(true); err == nil {
		t.Fatal("SetEnabled(true) succeeded despite the watcher failing")
	}
	if h.svc.Enabled() {
		t.Error("Enabled() = true after a failed start")
	}
	select {
	case err := <-h.errs:
		if err == nil {
			t.Error("nil error reported")
		}
	default:
		t.Error("start failure not reported")
	}
	select {
	case on := <-h.states:
		if on {
			t.Error("reported enabled after a failed start")
		}
	default:
	}
}

func TestService_DisableStopsWatching(t *testing.T) {
	h := newHarness(t, nil)
	mustEnable(t, h)
	<-h.states

	if err := h.svc.SetEnabled(false); err != nil {
		t.Fatalf("SetEnabled(false): %v", err)
	}
	if h.svc.Enabled() {
		t.Error("still enabled")
	}
	if on := <-h.states; on {
		t.Error("disable not reported")
	}
	h.source.mu.Lock()
	closed := h.source.closed
	h.source.mu.Unlock()
	if !closed {
		t.Error("watcher not closed on disable")
	}
}

func TestService_AppliesBatchAndRedrawsOnlyOnChange(t *testing.T) {
	h := newHarness(t, nil)
	h.target.result = func(paths []string) tikistore.ReloadResult {
		return tikistore.ReloadResult{Changed: paths[0] == "/changed.md"}
	}
	mustEnable(t, h)

	h.source.push("/unchanged.md")
	if got := recvPaths(t, h.target.reloaded); len(got) != 1 || got[0] != "/unchanged.md" {
		t.Fatalf("reloaded %v", got)
	}
	h.source.push("/changed.md")
	recvPaths(t, h.target.reloaded)

	// batches apply in order, so the unchanged one has finished by now
	select {
	case <-h.redrawn:
	case <-time.After(5 * time.Second):
		t.Fatal("changed batch never redrew")
	}
	if n := h.redraws.Load(); n != 1 {
		t.Errorf("redrew %d times, want 1 (only the batch that changed the store)", n)
	}
}

func TestService_InternalOnlyBatchNeverReachesUI(t *testing.T) {
	h := newHarness(t, nil)
	h.target.external = func(p string) bool { return p != "/self.md" }
	mustEnable(t, h)

	h.source.push("/self.md")
	waitChecked(t, h.target, "/self.md")
	h.source.push("/ext.md")
	got := recvPaths(t, h.target.reloaded)

	if len(got) != 1 || got[0] != "/ext.md" {
		t.Errorf("reloaded %v, want [/ext.md]", got)
	}
	if n := h.dispatch.Load(); n != 1 {
		t.Errorf("dispatched %d times, want 1 (the self-write must not reach the UI goroutine)", n)
	}
}

func waitChecked(t *testing.T, f *fakeTarget, path string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case paths := <-f.checked:
			for _, p := range paths {
				if p == path {
					return
				}
			}
		case <-deadline:
			t.Fatalf("%s never reached the pre-filter", path)
		}
	}
}

func TestService_HoldsBatchWhileBusy(t *testing.T) {
	h := newHarness(t, nil)
	mustEnable(t, h)
	h.busy.Store(true)

	h.source.push("/a.md")
	deadline := time.Now().Add(5 * time.Second)
	for h.dispatch.Load() < 2 { // first attempt plus at least one retry while busy
		if time.Now().After(deadline) {
			t.Fatal("no dispatch while busy")
		}
		time.Sleep(5 * time.Millisecond)
	}
	select {
	case p := <-h.target.reloaded:
		t.Fatalf("reloaded %v while busy", p)
	default:
	}

	h.busy.Store(false)
	if got := recvPaths(t, h.target.reloaded); len(got) != 1 || got[0] != "/a.md" {
		t.Errorf("after busy cleared, reloaded %v", got)
	}
}

func TestService_StaleDispatchIsDropped(t *testing.T) {
	queued := make(chan func(), 4)
	h := newHarness(t, func(o *Options) {
		o.Dispatch = func(fn func()) { queued <- fn }
	})
	mustEnable(t, h)

	h.source.push("/a.md")
	var fn func()
	select {
	case fn = <-queued:
	case <-time.After(5 * time.Second):
		t.Fatal("nothing dispatched")
	}
	if err := h.svc.SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	fn() // runs after live reload was turned off

	select {
	case p := <-h.target.reloaded:
		t.Errorf("stale dispatch reloaded %v", p)
	default:
	}
}

func TestService_StopInsideDispatchDoesNotDeadlock(t *testing.T) {
	var svc *Service
	h := newHarness(t, func(o *Options) {
		o.Dispatch = func(fn func()) {
			svc.Stop()
			fn()
		}
	})
	svc = h.svc
	mustEnable(t, h)

	h.source.push("/a.md")
	deadline := time.Now().Add(5 * time.Second)
	for h.svc.Enabled() {
		if time.Now().After(deadline) {
			t.Fatal("Stop inside dispatch never took effect")
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.svc.Stop() // idempotent
}

func TestService_RetryPathsAreReoffered(t *testing.T) {
	h := newHarness(t, nil)
	var first atomic.Bool
	first.Store(true)
	h.target.result = func(paths []string) tikistore.ReloadResult {
		if first.CompareAndSwap(true, false) {
			return tikistore.ReloadResult{Retry: paths}
		}
		return tikistore.ReloadResult{}
	}
	mustEnable(t, h)

	h.source.push("/mid-write.md")
	recvPaths(t, h.target.reloaded)
	if got := recvPaths(t, h.target.reloaded); len(got) != 1 || got[0] != "/mid-write.md" {
		t.Errorf("retry reloaded %v, want [/mid-write.md]", got)
	}
}

func TestService_ReconcileRequestReconciles(t *testing.T) {
	h := newHarness(t, nil)
	mustEnable(t, h)

	h.source.mu.Lock()
	b := h.source.batch
	h.source.mu.Unlock()
	b.requestReconcile()

	select {
	case <-h.target.reconciled:
	case <-time.After(5 * time.Second):
		t.Fatal("reconcile request not honoured")
	}
}

type countingStore struct {
	*tikistore.TikiStore
	reloaded chan []string
}

func (c *countingStore) ReloadPaths(paths []string) (tikistore.ReloadResult, error) {
	res, err := c.TikiStore.ReloadPaths(paths)
	c.reloaded <- paths
	return res, err
}

func TestService_InAppWritesAreNotEchoed(t *testing.T) {
	root := t.TempDir()
	pathA := filepath.Join(root, "a.md")
	write(t, pathA, "---\nid: AAAAAA\ntitle: a\n---\nbody\n")
	store, err := tikistore.NewTikiStore(root)
	if err != nil {
		t.Fatalf("NewTikiStore: %v", err)
	}
	target := &countingStore{TikiStore: store, reloaded: make(chan []string, 16)}

	var dispatched atomic.Int32
	svc := New(Options{
		Root:   root,
		Target: target,
		Dispatch: func(fn func()) {
			dispatched.Add(1)
			fn()
		},
		Quiet:    20 * time.Millisecond,
		MaxDelay: 200 * time.Millisecond,
	})
	t.Cleanup(svc.Stop)
	if err := svc.SetEnabled(true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}

	edited := store.GetTiki("AAAAAA").Clone()
	edited.SetTitle("edited in app")
	if err := store.UpdateTiki(edited); err != nil {
		t.Fatalf("UpdateTiki: %v", err)
	}
	pathB := filepath.Join(root, "b.md")
	write(t, pathB, "---\nid: BBBBBB\ntitle: b\n---\nbody\n")

	got := recvPaths(t, target.reloaded)
	if len(got) != 1 || got[0] != pathB {
		t.Errorf("reloaded %v, want only %s", got, pathB)
	}
	if n := dispatched.Load(); n != 1 {
		t.Errorf("dispatched %d times, want 1", n)
	}
	if store.GetTiki("BBBBBB") == nil {
		t.Error("external file not loaded")
	}
	if _, err := os.Stat(pathA); err != nil {
		t.Fatal(err)
	}
}

func TestService_FailedBatchRedrawsSoTheErrorShows(t *testing.T) {
	h := newHarness(t, nil)
	h.target.reloadErr = errors.New("disk on fire")
	mustEnable(t, h)

	h.source.push("/a.md")
	recvPaths(t, h.target.reloaded)

	select {
	case <-h.redrawn:
	case <-time.After(5 * time.Second):
		t.Fatal("no redraw after a failed batch: the error would only show on the next keypress")
	}
	select {
	case err := <-h.errs:
		if !strings.Contains(err.Error(), "disk on fire") {
			t.Errorf("reported %v", err)
		}
	default:
		t.Error("error not reported")
	}
}

func newSidecarHarness(t *testing.T) (*harness, *atomic.Int32) {
	t.Helper()
	var changes atomic.Int32
	h := newHarness(t, func(o *Options) {
		o.Sidecar = &Sidecar{Dir: filepath.Join(o.Root, ".tiki", "live"), OnChange: func() { changes.Add(1) }}
	})
	return h, &changes
}

func waitCount(t *testing.T, n *atomic.Int32, want int32) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for n.Load() < want {
		if time.Now().After(deadline) {
			t.Fatalf("count = %d, want %d", n.Load(), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestService_SidecarChangeSkipsTheStore(t *testing.T) {
	h, changes := newSidecarHarness(t)
	mustEnable(t, h)

	h.source.pushSide("/root/.tiki/live/AAAAAA.yaml")
	waitCount(t, changes, 1)
	waitCount(t, &h.redraws, 1)
	select {
	case p := <-h.target.reloaded:
		t.Errorf("sidecar change reloaded the store: %v", p)
	default:
	}
}

func TestService_SidecarChangeIsNotHeldWhileBusy(t *testing.T) {
	h, changes := newSidecarHarness(t)
	mustEnable(t, h)
	h.busy.Store(true)

	h.source.pushSide("/root/.tiki/live/AAAAAA.yaml")
	waitCount(t, changes, 1)
}

func TestService_StaleSidecarDispatchIsDropped(t *testing.T) {
	queued := make(chan func(), 4)
	var changes atomic.Int32
	h := newHarness(t, func(o *Options) {
		o.Dispatch = func(fn func()) { queued <- fn }
		o.Sidecar = &Sidecar{Dir: filepath.Join(o.Root, ".tiki", "live"), OnChange: func() { changes.Add(1) }}
	})
	mustEnable(t, h)

	h.source.pushSide("/root/.tiki/live/AAAAAA.yaml")
	var fn func()
	select {
	case fn = <-queued:
	case <-time.After(5 * time.Second):
		t.Fatal("nothing dispatched")
	}
	if err := h.svc.SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	fn()
	if changes.Load() != 0 {
		t.Error("sidecar change applied after live reload was turned off")
	}
}
