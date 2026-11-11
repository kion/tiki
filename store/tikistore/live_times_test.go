package tikistore

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/boolean-maybe/tiki/internal/livestatus"
)

func writeLive(t *testing.T, root string, at time.Time) {
	t.Helper()
	if err := livestatus.Write(root, "AAAAAA", livestatus.Status{Message: "Working", UpdatedAt: at}); err != nil {
		t.Fatalf("write live status: %v", err)
	}
}

// the sidecar format keeps whole seconds
func recentLiveTime() time.Time {
	return time.Now().Add(-time.Minute).Truncate(time.Second)
}

func TestLiveTimes_LoadedWithTheStore(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, filepath.Join(root, "a.md"), "AAAAAA", "a")
	at := recentLiveTime()
	writeLive(t, root, at)

	s, _, _ := newReloadStore(t, root)
	if got := s.GetTiki("AAAAAA").LiveAt(); !got.Equal(at) {
		t.Fatalf("LiveAt() = %v, want %v", got, at)
	}
}

func TestReloadLive_ReplacesChangedTikisAndNotifies(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, filepath.Join(root, "a.md"), "AAAAAA", "a")
	writeDoc(t, filepath.Join(root, "b.md"), "BBBBBB", "b")
	s, _, notified := newReloadStore(t, root)
	before, untouched := s.GetTiki("AAAAAA"), s.GetTiki("BBBBBB")

	at := recentLiveTime()
	writeLive(t, root, at)
	res := s.ReloadLive()
	if !res.Changed || len(res.Updated) != 1 || res.Updated[0] != "AAAAAA" {
		t.Fatalf("ReloadLive() = %+v, want AAAAAA updated", res)
	}
	if *notified != 1 {
		t.Fatalf("listeners notified %d times, want 1", *notified)
	}
	if got := s.GetTiki("AAAAAA").LiveAt(); !got.Equal(at) {
		t.Fatalf("LiveAt() = %v, want %v", got, at)
	}
	if !before.LiveAt().IsZero() {
		t.Fatal("ReloadLive changed a tiki already handed out")
	}
	if s.GetTiki("BBBBBB") != untouched {
		t.Fatal("ReloadLive replaced a tiki whose live status didn't change")
	}

	if res := s.ReloadLive(); res.Changed {
		t.Fatalf("ReloadLive() with nothing new = %+v", res)
	}
	if *notified != 1 {
		t.Fatal("listeners notified when nothing changed")
	}

	if err := livestatus.Remove(root, "AAAAAA"); err != nil {
		t.Fatal(err)
	}
	if res := s.ReloadLive(); !res.Changed || !s.GetTiki("AAAAAA").LiveAt().IsZero() {
		t.Fatalf("cleared status: ReloadLive() = %+v, LiveAt() = %v", res, s.GetTiki("AAAAAA").LiveAt())
	}
}

func TestLiveTimes_SurviveReloadsAndSaves(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.md")
	writeDoc(t, path, "AAAAAA", "a")
	at := recentLiveTime()
	writeLive(t, root, at)
	s, _, _ := newReloadStore(t, root)

	writeExternal(t, path, docContent("AAAAAA", "edited elsewhere"))
	mustReload(t, s, path)
	if got := s.GetTiki("AAAAAA").LiveAt(); !got.Equal(at) {
		t.Fatalf("after ReloadPaths: LiveAt() = %v, want %v", got, at)
	}

	writeExternal(t, path, docContent("AAAAAA", "edited again"))
	if _, err := s.Reconcile(); err != nil {
		t.Fatal(err)
	}
	if got := s.GetTiki("AAAAAA").LiveAt(); !got.Equal(at) {
		t.Fatalf("after Reconcile: LiveAt() = %v, want %v", got, at)
	}

	if err := s.ReloadTiki("AAAAAA"); err != nil {
		t.Fatal(err)
	}
	if got := s.GetTiki("AAAAAA").LiveAt(); !got.Equal(at) {
		t.Fatalf("after ReloadTiki: LiveAt() = %v, want %v", got, at)
	}

	edit := s.GetTiki("AAAAAA").Clone()
	edit.SetLiveAt(time.Time{})
	edit.SetTitle("edited in app")
	if err := s.UpdateTiki(edit); err != nil {
		t.Fatal(err)
	}
	if got := s.GetTiki("AAAAAA").LiveAt(); !got.Equal(at) {
		t.Fatalf("after UpdateTiki: LiveAt() = %v, want %v", got, at)
	}
}

func TestReconcile_ReadsNewLiveStatuses(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, filepath.Join(root, "a.md"), "AAAAAA", "a")
	s, _, notified := newReloadStore(t, root)

	at := recentLiveTime()
	writeLive(t, root, at)
	res, err := s.Reconcile()
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || *notified != 1 {
		t.Fatalf("Reconcile() = %+v, notified %d; want a change", res, *notified)
	}
	if got := s.GetTiki("AAAAAA").LiveAt(); !got.Equal(at) {
		t.Fatalf("LiveAt() = %v, want %v", got, at)
	}
}
