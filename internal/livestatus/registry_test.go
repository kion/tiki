package livestatus

import (
	"os"
	"testing"
	"time"
)

var registryNow = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

func newTestRegistry(t *testing.T, root string, anim *Animator) (*Registry, *[]Problem) {
	t.Helper()
	var warned []Problem
	r := NewRegistry(RegistryOptions{
		Root:       root,
		StaleAfter: 15 * time.Minute,
		Now:        func() time.Time { return registryNow },
		Animator:   anim,
		Warn:       func(p Problem) { warned = append(warned, p) },
	})
	return r, &warned
}

func writeStatus(t *testing.T, root string) {
	t.Helper()
	if err := Write(root, "X7F4K2", Status{Agent: "✻", Message: "Researching", UpdatedAt: registryNow.Add(-95 * time.Second)}); err != nil {
		t.Fatal(err)
	}
}

func TestRegistry_HiddenUntilEnabled(t *testing.T) {
	root := t.TempDir()
	writeStatus(t, root)
	r, _ := newTestRegistry(t, root, nil)

	if _, ok := r.Line("X7F4K2"); ok {
		t.Fatal("status shown while live reload is off")
	}
	r.SetEnabled(true)
	line, ok := r.Line("X7F4K2")
	if !ok || line.Message != "Researching" || line.Age != "1m" || line.Agent != "✻" {
		t.Fatalf("Line = %+v, %v", line, ok)
	}
}

func TestRegistry_RescanFollowsTheDirectory(t *testing.T) {
	root := t.TempDir()
	r, _ := newTestRegistry(t, root, nil)
	r.SetEnabled(true)

	writeStatus(t, root)
	r.Rescan()
	if _, ok := r.Line("X7F4K2"); !ok {
		t.Fatal("new status not picked up")
	}
	if err := Remove(root, "X7F4K2"); err != nil {
		t.Fatal(err)
	}
	r.Rescan()
	if _, ok := r.Line("X7F4K2"); ok {
		t.Fatal("cleared status still shown")
	}
}

func TestRegistry_DisableHidesAndStopsAnimating(t *testing.T) {
	root := t.TempDir()
	writeStatus(t, root)
	h := newAnimatorHarness()
	r, _ := newTestRegistry(t, root, h.anim)
	r.SetEnabled(true)
	r.Drawn(time.Second)
	ft := h.timer(t)

	r.SetEnabled(false)
	if !ft.stopped {
		t.Fatal("timer still armed after live reload turned off")
	}
	if _, ok := r.Line("X7F4K2"); ok {
		t.Fatal("status shown after live reload turned off")
	}
}

func TestRegistry_WarnsOncePerBrokenFileVersion(t *testing.T) {
	root := t.TempDir()
	writeStatus(t, root)
	broken := Path(root, "X7F4K2")
	if err := os.WriteFile(broken, []byte("message: [unclosed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, warned := newTestRegistry(t, root, nil)
	r.SetEnabled(true)
	r.Rescan()
	if len(*warned) != 1 {
		t.Fatalf("warnings = %d, want 1", len(*warned))
	}

	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(broken, later, later); err != nil {
		t.Fatal(err)
	}
	r.Rescan()
	if len(*warned) != 2 {
		t.Fatalf("warnings = %d, want 2 after the file changed", len(*warned))
	}
}

func TestRegistry_Clear(t *testing.T) {
	root := t.TempDir()
	writeStatus(t, root)
	r, _ := newTestRegistry(t, root, nil)
	r.SetEnabled(true)

	cleared, err := r.Clear("X7F4K2")
	if err != nil || !cleared {
		t.Fatalf("Clear = %v, %v; want true, nil", cleared, err)
	}
	if _, ok := r.Line("X7F4K2"); ok {
		t.Fatal("status still shown after Clear")
	}
	if _, ok, _ := Read(root, "X7F4K2"); ok {
		t.Fatal("status file still on disk after Clear")
	}

	cleared, err = r.Clear("X7F4K2")
	if err != nil || cleared {
		t.Fatalf("Clear of an absent status = %v, %v; want false, nil", cleared, err)
	}
}
