package livereload

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"

	"github.com/boolean-maybe/tiki/document"
)

// fsnotify isn't recursive, hence one watch per in-scope directory.
// All state belongs to the event loop; only Close is safe from other goroutines.
// Never log per event:
// the log file may sit under the root and would feed itself.
type dirWatcher struct {
	w       *fsnotify.Watcher
	root    string
	scope   *document.Scope
	watched map[string]bool
	known   func() []string // a removed directory reports no per-file events
	out     *batcher

	// The sidecar directory is outside the scope (it is hidden),
	// so its watches live apart from the scope's and survive resync.
	side        *sidecarWatch
	sideWatched map[string]bool

	closeOnce sync.Once
	closeErr  error

	reportedErr    bool
	reportedAddErr bool
}

type sidecarWatch struct {
	dir string
	out *batcher
}

func startDirWatcher(root string, known func() []string, out *batcher, side *sidecarWatch) (*dirWatcher, error) {
	d, err := newDirWatcher(root, known, out, side)
	if err != nil {
		return nil, err
	}
	go d.loop()
	return d, nil
}

// Separate from startDirWatcher so tests can simulate lost events.
func newDirWatcher(root string, known func() []string, out *batcher, side *sidecarWatch) (*dirWatcher, error) {
	scope, err := document.NewScope(root)
	if err != nil {
		return nil, fmt.Errorf("loading ignore rules: %w", err)
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("creating file watcher: %w", err)
	}
	d := &dirWatcher{
		w:       w,
		root:    filepath.Clean(root),
		scope:   scope,
		watched: map[string]bool{},
		known:   known,
		out:     out,

		sideWatched: map[string]bool{},
	}
	if side != nil {
		d.side = &sidecarWatch{dir: filepath.Clean(side.dir), out: side.out}
	}
	if _, err := d.addTree(d.root); err != nil {
		_ = w.Close()
		return nil, err
	}
	d.watchSideChain()
	return d, nil
}

func (d *dirWatcher) Close() error {
	d.closeOnce.Do(func() { d.closeErr = d.w.Close() })
	return d.closeErr
}

func (d *dirWatcher) loop() {
	for {
		select {
		case ev, ok := <-d.w.Events:
			if !ok {
				return
			}
			d.handle(ev)
		case err, ok := <-d.w.Errors:
			if !ok {
				return
			}
			d.handleError(err)
		}
	}
}

func (d *dirWatcher) handleError(err error) {
	if !d.reportedErr {
		slog.Warn("live reload: file watcher error, reconciling with disk", "error", err)
		d.reportedErr = true
	}
	// events may have been lost: directories created meanwhile have no watch
	d.resync()
	d.out.requestReconcile()
	if d.side != nil {
		d.side.out.add(d.side.dir)
	}
}

func (d *dirWatcher) warnAddFailure(err error) {
	if d.reportedAddErr {
		return
	}
	d.reportedAddErr = true
	slog.Warn("live reload: cannot watch new directory; changes inside it will need Refresh", "error", err)
}

func (d *dirWatcher) handle(ev fsnotify.Event) {
	path := filepath.Clean(ev.Name)

	if d.handleSidecar(ev, path) {
		return
	}

	if d.scope.IsIgnoreFile(path) {
		if scope, err := document.NewScope(d.root); err == nil {
			d.scope = scope
		}
		d.resync()
		d.out.requestReconcile()
		return
	}

	if d.watched[path] && ev.Has(fsnotify.Remove|fsnotify.Rename) {
		// nested watches must go too, or they keep reporting stale names;
		// files below get no events of their own.
		// A rename's new name arrives as a Create.
		d.removeTree(path)
		d.out.add(d.knownUnder(path)...)
		return
	}

	if ev.Has(fsnotify.Create) {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			if d.scope.IncludeDir(path) {
				files, err := d.addTree(path)
				if err != nil {
					d.warnAddFailure(err)
				}
				d.out.add(files...)
			}
			return
		}
	}

	// Chmod too: Linux reports deleting a still-open file that way
	if d.scope.IncludeFile(path) {
		d.out.add(path)
	}
}

// Each directory is watched before it is listed,
// so a file created meanwhile is either listed or reported by its own event.
func (d *dirWatcher) addTree(dir string) ([]string, error) {
	var files []string
	var firstErr error
	_ = d.scope.Walk(dir, func(path string, isDir bool) error {
		if !isDir {
			files = append(files, path)
			return nil
		}
		if d.watched[path] {
			return nil
		}
		if err := d.w.Add(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return filepath.SkipDir
			}
			if firstErr == nil {
				firstErr = explainWatchErr(fmt.Errorf("watching %s: %w", path, err))
			}
			return filepath.SkipDir
		}
		d.watched[path] = true
		return nil
	})
	return files, firstErr
}

// Errors are ignored:
// the backend may already have dropped the watch of a directory that is gone.
func (d *dirWatcher) removeTree(dir string) {
	prefix := dir + string(filepath.Separator)
	for p := range d.watched {
		if p == dir || strings.HasPrefix(p, prefix) {
			_ = d.w.Remove(p)
			delete(d.watched, p)
		}
	}
}

func (d *dirWatcher) resync() {
	want := map[string]bool{}
	_ = d.scope.Walk(d.root, func(path string, isDir bool) error {
		if isDir {
			want[path] = true
		}
		return nil
	})
	for p := range d.watched {
		if !want[p] {
			_ = d.w.Remove(p)
			delete(d.watched, p)
		}
	}
	for p := range want {
		if d.watched[p] {
			continue
		}
		if err := d.w.Add(p); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				d.warnAddFailure(explainWatchErr(err))
			}
			continue
		}
		d.watched[p] = true
	}
	d.watchSideChain()
}

func (d *dirWatcher) handleSidecar(ev fsnotify.Event, path string) bool {
	if d.side == nil {
		return false
	}
	parent := filepath.Dir(path)
	if parent == d.side.dir {
		d.side.out.add(path)
		return true
	}
	if !d.onSideChain(path) {
		// consumed, so state files such as .tiki/.gitignore reach neither batch
		return d.sideWatched[parent]
	}
	if ev.Has(fsnotify.Remove | fsnotify.Rename) {
		d.unwatchSide(path)
	} else if ev.Has(fsnotify.Create) {
		d.watchSideChain()
	}
	// files already in a directory that just appeared get no events of their own
	d.side.out.add(path)
	return true
}

func (d *dirWatcher) onSideChain(path string) bool {
	sep := string(filepath.Separator)
	if !strings.HasPrefix(path, d.root+sep) {
		return false
	}
	return path == d.side.dir || strings.HasPrefix(d.side.dir, path+sep)
}

// Each level is watched before the next is looked for,
// so a directory created meanwhile is either found or reported by its own event.
func (d *dirWatcher) watchSideChain() {
	if d.side == nil {
		return
	}
	rel, err := filepath.Rel(d.root, d.side.dir)
	if err != nil || strings.HasPrefix(rel, "..") {
		return
	}
	dir := d.root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		dir = filepath.Join(dir, part)
		if d.watched[dir] || d.sideWatched[dir] {
			continue
		}
		if err := d.w.Add(dir); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				d.warnAddFailure(explainWatchErr(err))
			}
			return
		}
		d.sideWatched[dir] = true
	}
}

func (d *dirWatcher) unwatchSide(dir string) {
	prefix := dir + string(filepath.Separator)
	for p := range d.sideWatched {
		if p == dir || strings.HasPrefix(p, prefix) {
			_ = d.w.Remove(p)
			delete(d.sideWatched, p)
		}
	}
}

func (d *dirWatcher) knownUnder(dir string) []string {
	prefix := dir + string(filepath.Separator)
	var out []string
	for _, p := range d.known() {
		if strings.HasPrefix(p, prefix) {
			out = append(out, p)
		}
	}
	return out
}
