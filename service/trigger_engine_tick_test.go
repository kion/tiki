package service

import (
	"context"
	"sync"
	"testing"

	"github.com/boolean-maybe/ruki"
	"github.com/boolean-maybe/tiki/store"
	"github.com/boolean-maybe/tiki/store/tikistore"
	tikipkg "github.com/boolean-maybe/tiki/tiki"
)

// dispatchProbe is a synchronous dispatcher that records whether code is
// running inside it (i.e. on the UI goroutine in the app).
type dispatchProbe struct {
	mu     sync.Mutex
	inside bool
	calls  int
}

func (p *dispatchProbe) dispatch(fn func()) {
	p.mu.Lock()
	p.inside = true
	p.calls++
	p.mu.Unlock()
	fn()
	p.mu.Lock()
	p.inside = false
	p.mu.Unlock()
}

func (p *dispatchProbe) isInside() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.inside
}

// snapshotProbeStore records, for each GetAllTikis call, whether it happened
// inside the dispatcher.
type snapshotProbeStore struct {
	*store.InMemoryStore
	probe  *dispatchProbe
	mu     sync.Mutex
	inside []bool
}

func (s *snapshotProbeStore) GetAllTikis() []*tikipkg.Tiki {
	s.mu.Lock()
	s.inside = append(s.inside, s.probe.isInside())
	s.mu.Unlock()
	return s.InMemoryStore.GetAllTikis()
}

func newTickEngine(t *testing.T, rule string, gate *TikiMutationGate) *TriggerEngine {
	t.Helper()
	tt, err := ruki.NewParser(testTriggerSchema{}).ParseTimeTrigger(rule)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	engine := NewTriggerEngine(nil, []TimeTriggerEntry{{Description: "tick", Trigger: tt}},
		ruki.NewTriggerExecutor(testTriggerSchema{}, testTriggerDocFactory(), nil))
	engine.RegisterWithGate(gate)
	return engine
}

func TestTimeTriggerTick_NoMatchesNeverReachesUI(t *testing.T) {
	gate, _ := newGateWithStoreAndTikis(newTiki("ACT001", "active", "ready", "story", 3))
	engine := newTickEngine(t, `every 1sec delete where status = "done"`, gate)
	probe := &dispatchProbe{}
	engine.SetDispatch(probe.dispatch)

	engine.runTick(context.Background(), engine.timeTriggers[0])

	if probe.calls != 0 {
		t.Errorf("a tick that changes nothing dispatched %d times, want 0", probe.calls)
	}
}

func TestTimeTriggerTick_EvaluatesOffUIAndAppliesOnUI(t *testing.T) {
	probe := &dispatchProbe{}
	ps := &snapshotProbeStore{InMemoryStore: store.NewInMemoryStore(), probe: probe}
	gate := NewTikiMutationGate()
	RegisterFieldValidators(gate)
	gate.SetStore(ps)
	if err := gate.CreateTiki(context.Background(), newTiki("DONE01", "done", "done", "story", 3)); err != nil {
		t.Fatal(err)
	}
	engine := newTickEngine(t, `every 1sec delete where status = "done"`, gate)
	engine.SetDispatch(probe.dispatch)
	ps.mu.Lock()
	ps.inside = nil
	ps.mu.Unlock()

	engine.runTick(context.Background(), engine.timeTriggers[0])

	if ps.GetTiki("DONE01") != nil {
		t.Fatal("tick did not apply its result")
	}
	if probe.calls != 1 {
		t.Errorf("dispatched %d times, want 1 (apply only)", probe.calls)
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if len(ps.inside) == 0 || ps.inside[0] {
		t.Errorf("trigger evaluation read the store inside the dispatcher (UI goroutine): %v", ps.inside)
	}
}

func TestTimeTriggerTick_AppliesWithOneNotification(t *testing.T) {
	gate, s := newGateWithStoreAndTikis(
		newTiki("DONE01", "a", "done", "story", 3),
		newTiki("DONE02", "b", "done", "story", 3),
		newTiki("DONE03", "c", "done", "story", 3),
	)
	engine := newTickEngine(t, `every 1sec delete where status = "done"`, gate)
	engine.SetDispatch((&dispatchProbe{}).dispatch)
	notified := 0
	s.AddListener(func() { notified++ })

	engine.runTick(context.Background(), engine.timeTriggers[0])

	if len(s.GetAllTikis()) != 0 {
		t.Fatalf("tick left %d tikis", len(s.GetAllTikis()))
	}
	if notified != 1 {
		t.Errorf("a tick changing 3 tikis notified listeners %d times, want 1 (one re-render)", notified)
	}
}

// TestTimeTriggerTick_StaleResultDoesNotOverwriteEdit covers the window
// between evaluating a tick off the UI goroutine and applying it there: a
// user edit landing in between must survive. The evaluated copies carry the
// file state they were read from, so the store's save check rejects them.
func TestTimeTriggerTick_StaleResultDoesNotOverwriteEdit(t *testing.T) {
	s, err := tikistore.NewTikiStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewTikiStore: %v", err)
	}
	gate := NewTikiMutationGate()
	RegisterFieldValidators(gate)
	gate.SetStore(s)
	if err := gate.CreateTiki(context.Background(), newTiki("READY1", "original", "ready", "story", 3)); err != nil {
		t.Fatalf("create: %v", err)
	}
	engine := newTickEngine(t, `every 1sec update where status = "ready" set title = "set by trigger"`, gate)
	engine.SetDispatch(func(apply func()) {
		// the user edits the tiki after the tick was evaluated, before it applies
		edited := s.GetTiki("READY1").Clone()
		edited.SetTitle("edited by user")
		if err := gate.UpdateTiki(context.Background(), edited); err != nil {
			t.Fatalf("user edit: %v", err)
		}
		apply()
	})

	engine.runTick(context.Background(), engine.timeTriggers[0])

	if got := s.GetTiki("READY1").Title(); got != "edited by user" {
		t.Errorf("title = %q: the stale trigger result overwrote the user's edit", got)
	}
}
