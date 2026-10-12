package livereload

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

type collected struct {
	paths     map[string]bool
	reconcile bool
}

// Event timing relative to the test isn't guaranteed,
// so assert on everything collected rather than on single batches.
func waitFor(t *testing.T, b *batcher, acc *collected, pred func(*collected) bool) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for !pred(acc) {
		select {
		case <-b.ready():
			paths, reconcile := b.take()
			for _, p := range paths {
				acc.paths[p] = true
			}
			acc.reconcile = acc.reconcile || reconcile
		case <-deadline:
			t.Fatalf("timed out; collected paths %v, reconcile %v", keysOf(acc.paths), acc.reconcile)
		}
	}
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func has(path string) func(*collected) bool {
	return func(c *collected) bool { return c.paths[path] }
}

type knownPaths struct {
	mu    sync.Mutex
	paths []string
}

func (k *knownPaths) set(paths ...string) {
	k.mu.Lock()
	k.paths = paths
	k.mu.Unlock()
}

func (k *knownPaths) get() []string {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]string(nil), k.paths...)
}

// Registered after t.TempDir's cleanup, so it runs first:
// Windows can't remove a directory that still has a watch handle.
func startTestWatcher(t *testing.T, root string, known *knownPaths) (*batcher, *collected) {
	t.Helper()
	b := newBatcher(20*time.Millisecond, 200*time.Millisecond)
	if known == nil {
		known = &knownPaths{}
	}
	w, err := startDirWatcher(root, known.get, b, nil)
	if err != nil {
		t.Fatalf("startDirWatcher: %v", err)
	}
	t.Cleanup(func() {
		_ = w.Close()
		b.stop()
	})
	return b, &collected{paths: map[string]bool{}}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	//nolint:gosec // G301: test temp dir
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // G306: test temp file
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWatcher_ReportsDocumentWrite(t *testing.T) {
	root := t.TempDir()
	b, acc := startTestWatcher(t, root, nil)

	p := filepath.Join(root, "a.md")
	write(t, p, "x")
	waitFor(t, b, acc, has(p))
}

func TestWatcher_BurstOfWrites(t *testing.T) {
	root := t.TempDir()
	b, acc := startTestWatcher(t, root, nil)

	var want []string
	for i := 0; i < 20; i++ {
		p := filepath.Join(root, "doc"+string(rune('a'+i))+".md")
		want = append(want, p)
		write(t, p, "x")
	}
	waitFor(t, b, acc, func(c *collected) bool {
		for _, p := range want {
			if !c.paths[p] {
				return false
			}
		}
		return true
	})
}

func TestWatcher_IgnoresNonDocuments(t *testing.T) {
	root := t.TempDir()
	b, acc := startTestWatcher(t, root, nil)

	write(t, filepath.Join(root, "tiki.log"), "noise")
	write(t, filepath.Join(root, ".hidden", "x.md"), "hidden dir")
	sentinel := filepath.Join(root, "sentinel.md")
	write(t, sentinel, "x")
	waitFor(t, b, acc, has(sentinel))

	for p := range acc.paths {
		if p != sentinel {
			t.Errorf("unexpected path reported: %s", p)
		}
	}
}

func TestWatcher_NewDirectoryThenImmediateWrite(t *testing.T) {
	root := t.TempDir()
	b, acc := startTestWatcher(t, root, nil)

	// written before the new directory can get a watch
	p := filepath.Join(root, "sub", "x.md")
	write(t, p, "x")
	waitFor(t, b, acc, has(p))

	p2 := filepath.Join(root, "sub", "y.md")
	write(t, p2, "y")
	waitFor(t, b, acc, has(p2))
}

func TestWatcher_DeepNewTree(t *testing.T) {
	root := t.TempDir()
	b, acc := startTestWatcher(t, root, nil)

	p := filepath.Join(root, "a", "b", "c", "x.md")
	write(t, p, "x")
	waitFor(t, b, acc, has(p))
}

func TestWatcher_DirectoryRename(t *testing.T) {
	root := t.TempDir()
	oldDoc := filepath.Join(root, "d", "e", "x.md")
	write(t, oldDoc, "x")
	known := &knownPaths{}
	known.set(oldDoc)
	b, acc := startTestWatcher(t, root, known)

	if err := os.Rename(filepath.Join(root, "d"), filepath.Join(root, "f")); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(root, "f", "e", "x.md")
	waitFor(t, b, acc, func(c *collected) bool { return c.paths[oldDoc] && c.paths[moved] })

	later := filepath.Join(root, "f", "e", "y.md")
	write(t, later, "y")
	waitFor(t, b, acc, has(later))
	if acc.paths[filepath.Join(root, "d", "e", "y.md")] {
		t.Error("write reported under the pre-rename path")
	}
}

func TestWatcher_DirectoryRemoval(t *testing.T) {
	root := t.TempDir()
	doc := filepath.Join(root, "d", "x.md")
	write(t, doc, "x")
	known := &knownPaths{}
	known.set(doc)
	b, acc := startTestWatcher(t, root, known)

	if err := os.RemoveAll(filepath.Join(root, "d")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, b, acc, has(doc))
}

func TestWatcher_IgnoreFileChangeRequestsReconcileAndRescopes(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "ign", "keep.md"), "x")
	b, acc := startTestWatcher(t, root, nil)

	write(t, filepath.Join(root, ".tikiignore"), "ign/\n")
	waitFor(t, b, acc, func(c *collected) bool { return c.reconcile })

	write(t, filepath.Join(root, "ign", "z.md"), "z")
	sentinel := filepath.Join(root, "sentinel.md")
	write(t, sentinel, "x")
	waitFor(t, b, acc, has(sentinel))
	if acc.paths[filepath.Join(root, "ign", "z.md")] {
		t.Error("write in newly ignored directory was reported")
	}
}

func TestWatcher_CloseIsIdempotent(t *testing.T) {
	root := t.TempDir()
	b := newBatcher(20*time.Millisecond, 200*time.Millisecond)
	w, err := startDirWatcher(root, func() []string { return nil }, b, nil)
	if err != nil {
		t.Fatalf("startDirWatcher: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestWatcher_ErrorResyncsMissedDirectories(t *testing.T) {
	root := t.TempDir()
	b := newBatcher(20*time.Millisecond, 200*time.Millisecond)
	// no event loop: the create event below is never handled, as if lost
	d, err := newDirWatcher(root, func() []string { return nil }, b, nil)
	if err != nil {
		t.Fatalf("newDirWatcher: %v", err)
	}
	t.Cleanup(func() {
		_ = d.Close()
		b.stop()
	})

	late := filepath.Join(root, "late", "deeper")
	//nolint:gosec // G301: test temp dir
	if err := os.MkdirAll(late, 0o755); err != nil {
		t.Fatal(err)
	}
	d.handleError(fsnotify.ErrEventOverflow)

	if !d.watched[filepath.Join(root, "late")] || !d.watched[late] {
		t.Errorf("directories created during lost events are unwatched: %v", keysOf(d.watched))
	}
	if _, reconcile := b.take(); !reconcile {
		t.Error("no reconcile requested after a watcher error")
	}
}

func startSidecarTestWatcher(t *testing.T, root string) (main, side *batcher, mainAcc, sideAcc *collected) {
	t.Helper()
	main = newBatcher(20*time.Millisecond, 200*time.Millisecond)
	side = newBatcher(20*time.Millisecond, 200*time.Millisecond)
	w, err := startDirWatcher(root, func() []string { return nil }, main,
		&sidecarWatch{dir: filepath.Join(root, ".tiki", "live"), out: side})
	if err != nil {
		t.Fatalf("startDirWatcher: %v", err)
	}
	t.Cleanup(func() {
		_ = w.Close()
		main.stop()
		side.stop()
	})
	return main, side, &collected{paths: map[string]bool{}}, &collected{paths: map[string]bool{}}
}

func anything(c *collected) bool { return len(c.paths) > 0 }

func assertNothingUnder(t *testing.T, acc *collected, dir string) {
	t.Helper()
	for p := range acc.paths {
		if p == dir || filepath.Dir(p) == dir || filepath.Dir(filepath.Dir(p)) == dir {
			t.Errorf("unexpected path reported: %s", p)
		}
	}
}

func TestWatcher_SidecarDirCreatedLater(t *testing.T) {
	root := t.TempDir()
	main, side, mainAcc, sideAcc := startSidecarTestWatcher(t, root)

	write(t, filepath.Join(root, ".tiki", "live", "AAAAAA.yaml"), "message: x\n")
	waitFor(t, side, sideAcc, anything)
	second := filepath.Join(root, ".tiki", "live", "BBBBBB.yaml")
	write(t, second, "message: y\n")
	waitFor(t, side, sideAcc, has(second))

	sentinel := filepath.Join(root, "sentinel.md")
	write(t, sentinel, "x")
	waitFor(t, main, mainAcc, has(sentinel))
	assertNothingUnder(t, mainAcc, filepath.Join(root, ".tiki"))
}

func TestWatcher_SidecarDirExistingAtStart(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".tiki", "live", "AAAAAA.yaml"), "message: x\n")
	_, side, _, sideAcc := startSidecarTestWatcher(t, root)

	p := filepath.Join(root, ".tiki", "live", "BBBBBB.yaml")
	write(t, p, "message: y\n")
	waitFor(t, side, sideAcc, has(p))
}

func TestWatcher_SidecarDirRecreated(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".tiki", "live", "AAAAAA.yaml"), "message: x\n")
	_, side, _, sideAcc := startSidecarTestWatcher(t, root)

	if err := os.RemoveAll(filepath.Join(root, ".tiki")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, side, sideAcc, anything)

	p := filepath.Join(root, ".tiki", "live", "CCCCCC.yaml")
	write(t, p, "message: z\n")
	waitFor(t, side, sideAcc, func(c *collected) bool {
		if c.paths[p] {
			return true
		}
		// the new file may land before its directory's watch; rewrite until an event shows up
		write(t, p, "message: z\n")
		return false
	})
}

func TestWatcher_SidecarIgnoresOtherStateFiles(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".tiki", "live", "AAAAAA.yaml"), "message: x\n")
	main, side, mainAcc, sideAcc := startSidecarTestWatcher(t, root)

	other := filepath.Join(root, ".tiki", ".gitignore")
	write(t, other, "*\n")
	p := filepath.Join(root, ".tiki", "live", "BBBBBB.yaml")
	write(t, p, "message: y\n")
	waitFor(t, side, sideAcc, has(p))
	sentinel := filepath.Join(root, "sentinel.md")
	write(t, sentinel, "x")
	waitFor(t, main, mainAcc, has(sentinel))

	if sideAcc.paths[other] || mainAcc.paths[other] {
		t.Errorf("%s was reported", other)
	}
}

func TestWatcher_ResyncKeepsSidecarWatches(t *testing.T) {
	root := t.TempDir()
	liveDir := filepath.Join(root, ".tiki", "live")
	write(t, filepath.Join(liveDir, "AAAAAA.yaml"), "message: x\n")
	main := newBatcher(20*time.Millisecond, 200*time.Millisecond)
	side := newBatcher(20*time.Millisecond, 200*time.Millisecond)
	// no event loop, so the test can call into the watcher directly
	d, err := newDirWatcher(root, func() []string { return nil }, main, &sidecarWatch{dir: liveDir, out: side})
	if err != nil {
		t.Fatalf("newDirWatcher: %v", err)
	}
	t.Cleanup(func() {
		_ = d.Close()
		main.stop()
		side.stop()
	})

	d.handleError(fsnotify.ErrEventOverflow)

	watching := false
	for _, p := range d.w.WatchList() {
		watching = watching || p == liveDir
	}
	if !watching {
		t.Errorf("%s unwatched after a resync: %v", liveDir, d.w.WatchList())
	}
	if paths, _ := side.take(); len(paths) == 0 {
		t.Error("no sidecar rescan requested after a watcher error")
	}
}
