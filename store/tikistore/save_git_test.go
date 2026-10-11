package tikistore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	tikipkg "github.com/boolean-maybe/tiki/tiki"
)

// TestSaveDoesNotQueryGitHistory pins that saving never runs a git history
// query. UpdatedAt is max(file mtime, last commit time), and a file that was
// just written is newer than any commit, so the lookup only cost a git
// subprocess per save — on the UI thread, once per tiki a trigger changes.
func TestSaveDoesNotQueryGitHistory(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, filepath.Join(root, "a.md"), "AAAAAA", "a")
	s, g, _ := newReloadStore(t, root)

	edited := s.GetTiki("AAAAAA").Clone()
	edited.SetTitle("edited in app")
	if err := s.UpdateTiki(edited); err != nil {
		t.Fatalf("UpdateTiki: %v", err)
	}
	created := tikipkg.New()
	created.SetTitle("created in app")
	if err := s.CreateTiki(created); err != nil {
		t.Fatalf("CreateTiki: %v", err)
	}

	if n := g.historyCalls(); n != 0 {
		t.Errorf("saving issued %d git history calls, want 0", n)
	}
	info, err := os.Stat(edited.Path())
	if err != nil {
		t.Fatal(err)
	}
	if got := s.GetTiki("AAAAAA").UpdatedAt(); !got.Equal(info.ModTime()) {
		t.Errorf("UpdatedAt = %v, want the saved file's mtime %v", got, info.ModTime())
	}
}

func TestBatchNotificationsCoalesces(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, filepath.Join(root, "a.md"), "AAAAAA", "a")
	writeDoc(t, filepath.Join(root, "b.md"), "BBBBBB", "b")
	s, _, notified := newReloadStore(t, root)

	s.BatchNotifications(func() {
		a := s.GetTiki("AAAAAA").Clone()
		a.SetTitle("a2")
		if err := s.UpdateTiki(a); err != nil {
			t.Fatalf("UpdateTiki: %v", err)
		}
		s.BatchNotifications(func() { // nested batches flush once, at the outermost end
			s.DeleteTiki("BBBBBB")
		})
		if *notified != 0 {
			t.Errorf("notified %d times inside the batch, want 0", *notified)
		}
	})
	if *notified != 1 {
		t.Errorf("notified %d times for the batch, want 1", *notified)
	}

	*notified = 0
	s.BatchNotifications(func() {})
	if *notified != 0 {
		t.Errorf("an empty batch notified %d times", *notified)
	}

	s.DeleteTiki("AAAAAA") // outside a batch: immediate, as before
	if *notified != 1 {
		t.Errorf("unbatched change notified %d times, want 1", *notified)
	}
}

// TestSaveDetectsConflictWithSameMtime covers filesystems with coarse
// timestamps (jiffy-granular on many Linux kernels, 2s on FAT): a competing
// write can leave the mtime unchanged, so the optimistic lock must also
// compare the size before overwriting.
func TestSaveDetectsConflictWithSameMtime(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.md")
	writeDoc(t, path, "AAAAAA", "a")
	s, _, _ := newReloadStore(t, root)
	stale := s.GetTiki("AAAAAA").Clone() // e.g. a trigger evaluated before the edit

	// a competing write lands in the same timestamp tick
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(docContent("AAAAAA", "a competing, longer edit")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}

	stale.SetTitle("stale write")
	if err := s.UpdateTiki(stale); !errors.Is(err, ErrConflict) {
		t.Fatalf("UpdateTiki over a same-mtime competing write = %v, want ErrConflict", err)
	}
}
