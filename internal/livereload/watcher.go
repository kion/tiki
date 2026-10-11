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

	closeOnce sync.Once
	closeErr  error

	reportedErr    bool
	reportedAddErr bool
}

func startDirWatcher(root string, known func() []string, out *batcher) (*dirWatcher, error) {
	d, err := newDirWatcher(root, known, out)
	if err != nil {
		return nil, err
	}
	go d.loop()
	return d, nil
}

// Separate from startDirWatcher so tests can simulate lost events.
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
