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

// dirWatcher turns fsnotify events under a document root into changed
// document paths. fsnotify is not recursive, so it registers one watch per
// directory the document scan would descend into (document.Scope), adds
// watches as directories appear and drops them as they go.
//
// All watcher state is owned by the event loop goroutine; only Close may be
// called from elsewhere.
//
// It never logs per event: the log file may live under the root, and a log
// line per event would feed itself.
type dirWatcher struct {
	w       *fsnotify.Watcher
	root    string
	scope   *document.Scope
	watched map[string]bool
	known   func() []string // current tiki paths, to expand directory removals
	out     *batcher

	closeOnce sync.Once
	closeErr  error

	reportedErr    bool // a backend error was logged; later ones stay quiet
	reportedAddErr bool // a watch registration failure was logged
}

// startDirWatcher registers watches for every in-scope directory under root
// and starts the event loop. A registration failure (e.g. the inotify watch
// limit or the open-file limit) is returned and nothing keeps running.
func startDirWatcher(root string, known func() []string, out *batcher) (*dirWatcher, error) {
	d, err := newDirWatcher(root, known, out)
	if err != nil {
		return nil, err
	}
	go d.loop()
	return d, nil
}

// newDirWatcher registers the watches without starting the event loop.
func newDirWatcher(root string, known func() []string, out *batcher) (*dirWatcher, error) {
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
	}
	if _, err := d.addTree(d.root); err != nil {
		_ = w.Close()
		return nil, err
	}
	return d, nil
}

// Close stops the watcher and its event loop. Idempotent.
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

// handleError recovers from a backend error such as a queue overflow, after
// which events may have been lost.
func (d *dirWatcher) handleError(err error) {
	if !d.reportedErr {
		slog.Warn("live reload: file watcher error, reconciling with disk", "error", err)
		d.reportedErr = true
	}
	// directories created while events were dropped have no watch yet, and
	// the whole tree is compared with the store.
	d.resync()
	d.out.requestReconcile()
}

// warnAddFailure logs, once per watcher, that a directory could not be
// watched (e.g. the watch or open-file limit was reached), so changes inside
// it will not be picked up live.
func (d *dirWatcher) warnAddFailure(err error) {
	if d.reportedAddErr {
		return
	}
	d.reportedAddErr = true
	slog.Warn("live reload: cannot watch new directory; changes inside it will need Refresh", "error", err)
}

func (d *dirWatcher) handle(ev fsnotify.Event) {
	path := filepath.Clean(ev.Name)

	if d.scope.IsIgnoreFile(path) {
		if scope, err := document.NewScope(d.root); err == nil {
			d.scope = scope
		}
		d.resync()
		d.out.requestReconcile()
		return
	}

	if d.watched[path] && ev.Has(fsnotify.Remove|fsnotify.Rename) {
		// a watched directory was removed or moved away: drop its watches
		// (nested ones too — they would keep reporting stale names) and let
		// the store re-check every tiki that lived below it. A rename's new
		// name arrives as its own Create.
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

	// any event on a document path (Chmod included: some platforms report a
	// delete of a still-open file that way) means "check this path"; the
	// store compares it with what it holds.
	if d.scope.IncludeFile(path) {
		d.out.add(path)
	}
}

// addTree watches dir and every in-scope directory below it, returning the
// documents found there. Each directory is watched before it is listed, so a
// file created concurrently is either listed or reported by its own event.
// Only the first registration error is returned; directories that vanish
// mid-walk are skipped.
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
				firstErr = fmt.Errorf("watching %s: %w", path, err)
			}
			return filepath.SkipDir
		}
		d.watched[path] = true
		return nil
	})
	return files, firstErr
}

// removeTree drops the watch on dir and on every watched directory below it.
// Errors are ignored: the backend may already have dropped a watch whose
// directory is gone.
func (d *dirWatcher) removeTree(dir string) {
	prefix := dir + string(filepath.Separator)
	for p := range d.watched {
		if p == dir || strings.HasPrefix(p, prefix) {
			_ = d.w.Remove(p)
			delete(d.watched, p)
		}
	}
}

// resync makes the watch set match the current scope after the ignore rules
// changed.
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
				d.warnAddFailure(err)
			}
			continue
		}
		d.watched[p] = true
	}
}

// knownUnder returns the loaded tiki paths below dir.
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
