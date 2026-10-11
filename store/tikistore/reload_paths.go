package tikistore

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/boolean-maybe/tiki/document"
	"github.com/boolean-maybe/tiki/tiki"
)

// ReloadResult reports what an incremental reload or Reconcile changed.
type ReloadResult struct {
	// Changed is true when any tiki was added, updated or removed. Listeners
	// are notified only when it is set.
	Changed bool
	// Added, Updated and Removed list the affected tiki ids, sorted.
	Added   []string
	Updated []string
	Removed []string
	// Retry lists paths left untouched because a known document read as
	// empty or unparsable, which usually means it was caught mid-write.
	// Offer them again in a later batch; the second sighting is final.
	Retry []string
	// Reconciled reports that the batch was resolved by a full git-free
	// Reconcile (id changes, duplicate ids, rejected files).
	Reconciled bool
}

// ReloadPaths re-reads the given paths after an external change and applies
// the result to the store without touching git. It is the live-reload
// counterpart of ReloadTiki: paths may be new, modified, deleted, renamed or
// no longer in scope, and are matched to tikis by file path.
//
// Simple cases (add, modify, delete, rename) are applied directly, keeping
// every unaffected tiki's pointer. Cases where the full-load rules decide the
// outcome — a frontmatter id change, an id collision, a file with a recorded
// load rejection, a persistent parse error — fall back to Reconcile so the
// result matches a fresh load. A known path whose mtime and size still match
// memory is skipped: that is how tiki's own writes are recognized.
//
// Git-derived metadata (createdAt, createdBy) is carried over from the
// in-memory tiki; files new to the store get the same file-metadata fallback
// a full load gives untracked files. Listeners are notified at most once.
func (s *TikiStore) ReloadPaths(paths []string) (ReloadResult, error) {
	scope, err := document.NewScope(s.dir)
	if err != nil {
		return ReloadResult{}, fmt.Errorf("loading ignore rules: %w", err)
	}

	s.mu.Lock()
	res, err := s.reloadPathsLocked(scope, paths)
	s.mu.Unlock()
	if err != nil {
		return res, err
	}
	if res.Changed {
		s.notifyListeners()
	}
	return res, nil
}

// Reconcile is a git-free equivalent of Reload: it walks the documents,
// re-parses only files whose mtime or size changed, carries git metadata over
// by path, and rebuilds the id index and diagnostics with the full-load rules.
// Unchanged tikis keep their pointers. Listeners are notified only on change.
func (s *TikiStore) Reconcile() (ReloadResult, error) {
	s.mu.Lock()
	res, err := s.reconcileLocked(nil)
	s.mu.Unlock()
	if err != nil {
		return res, err
	}
	if res.Changed {
		s.notifyListeners()
	}
	return res, nil
}

// ExternallyChanged returns the subset of paths whose on-disk state differs
// from what the store holds: known files whose mtime or size changed or that
// are gone, and existing files the store does not know. Paths the store wrote
// itself (and files it deleted itself) are filtered out, so live reload can
// drop the echo of in-app changes before touching the UI goroutine.
//
// Only stats files; safe to call from any goroutine. It holds the read lock,
// so it waits for an in-progress save to finish stamping LoadedMtime.
func (s *TikiStore) ExternallyChanged(paths []string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	byPath := s.tikisByPathLocked()

	var out []string
	for _, path := range normalizePaths(paths) {
		known := byPath[path]
		info, err := os.Stat(path)
		if err != nil {
			if known != nil {
				out = append(out, path)
			}
			continue
		}
		if info.IsDir() {
			continue
		}
		if known != nil && sameFileState(known, info) {
			continue
		}
		out = append(out, path)
	}
	return out
}

// pathUpsert is a parsed file waiting to be applied by reloadPathsLocked.
type pathUpsert struct {
	path   string
	old    *tiki.Tiki // tiki previously loaded from this path, or nil
	loaded *tiki.Tiki
}

func (s *TikiStore) reloadPathsLocked(scope *document.Scope, paths []string) (ReloadResult, error) {
	var res ReloadResult
	byPath := s.tikisByPathLocked()

	var removals []*tiki.Tiki
	var upserts []pathUpsert
	keep := map[string]bool{} // deferred paths a reconcile must leave as they are
	needReconcile := false

	for _, path := range normalizePaths(paths) {
		old := byPath[path]

		if !scope.IncludeFile(path) {
			// ignored, hidden or not markdown; a known tiki here left scope
			// (e.g. an ignore pattern now matches it).
			if old != nil {
				removals = append(removals, old)
			}
			continue
		}

		info, statErr := os.Stat(path)
		if statErr != nil || info.IsDir() {
			if old != nil {
				removals = append(removals, old)
			} else if s.diagnostics.hasPath(path) {
				needReconcile = true // clear the stale rejection
			}
			s.clearDeferral(path)
			continue
		}

		if old != nil && sameFileState(old, info) {
			s.clearDeferral(path)
			continue
		}

		loaded, loadErr := s.loadTikiFile(path, nil, nil)
		if loadErr != nil {
			if errors.Is(loadErr, errSkipNoID) && info.Size() > 0 {
				// plain wiki content; a known tiki whose file lost its id is
				// dropped silently, as a full load would.
				if old != nil {
					removals = append(removals, old)
				}
				s.clearDeferral(path)
				continue
			}
			// empty or unparsable: a known document is most likely being
			// rewritten (os.WriteFile truncates first). Give the writer one
			// more batch before treating the failure as real.
			if old != nil && s.deferOnce(path) {
				res.Retry = append(res.Retry, path)
				keep[path] = true
				continue
			}
			if old == nil && errors.Is(loadErr, errSkipNoID) && !s.diagnostics.hasPath(path) {
				continue // empty unknown file: nothing to do
			}
			needReconcile = true // record the rejection exactly like a full load
			continue
		}
		s.clearDeferral(path)

		if s.diagnostics.hasPath(path) || (old != nil && old.ID() != loaded.ID()) {
			needReconcile = true
			continue
		}
		upserts = append(upserts, pathUpsert{path: path, old: old, loaded: loaded})
	}

	if !needReconcile {
		needReconcile = s.needsReconcileForIDs(removals, upserts)
	}
	if needReconcile {
		rec, err := s.reconcileLocked(keep)
		if err != nil {
			return res, err
		}
		rec.Retry = res.Retry
		return rec, nil
	}

	removed := map[string]bool{}
	for _, old := range removals {
		if s.tikis[old.ID()] == old {
			delete(s.tikis, old.ID())
			removed[old.ID()] = true
		}
	}
	for _, u := range upserts {
		id := u.loaded.ID()
		if u.old != nil {
			carryForwardGitMeta(u.loaded, u.old)
		}
		s.tikis[id] = u.loaded
		if u.old != nil || removed[id] {
			// a rename (remove + add of one id) reports as an update
			delete(removed, id)
			res.Updated = append(res.Updated, id)
		} else {
			res.Added = append(res.Added, id)
		}
	}
	for id := range removed {
		res.Removed = append(res.Removed, id)
	}
	sort.Strings(res.Added)
	sort.Strings(res.Updated)
	sort.Strings(res.Removed)
	res.Changed = len(res.Added)+len(res.Updated)+len(res.Removed) > 0
	if res.Changed {
		slog.Info("tikis reloaded from external changes",
			"added", len(res.Added), "updated", len(res.Updated), "removed", len(res.Removed))
	}
	return res, nil
}

// needsReconcileForIDs reports whether applying the batch directly could
// break the id invariants: a new file claiming an id another live file holds
// (or two new files claiming one id), or a removal while a rejected duplicate
// may be waiting to take over the freed id.
func (s *TikiStore) needsReconcileForIDs(removals []*tiki.Tiki, upserts []pathUpsert) bool {
	if len(removals) > 0 && s.diagnostics.hasReason(LoadReasonDuplicateID) {
		return true
	}
	leaving := map[*tiki.Tiki]bool{}
	for _, old := range removals {
		leaving[old] = true
	}
	claimed := map[string]bool{}
	for _, u := range upserts {
		id := u.loaded.ID()
		if claimed[id] {
			return true
		}
		claimed[id] = true
		if u.old != nil {
			continue
		}
		if holder, taken := s.tikis[id]; taken && !leaving[holder] {
			return true
		}
	}
	return false
}

// reconcileLocked rebuilds the tiki map from disk without git. Files whose
// mtime and size match the loaded tiki at the same path are reused as-is, as
// are paths in keep (deferred mid-write files). Caller must hold s.mu.
func (s *TikiStore) reconcileLocked(keep map[string]bool) (ReloadResult, error) {
	docPaths, err := s.collectDocumentPaths()
	if err != nil {
		return ReloadResult{}, fmt.Errorf("walking directory: %w", err)
	}

	byPath := s.tikisByPathLocked()
	next := make(map[string]*tiki.Tiki, len(s.tikis))
	diags := newLoadDiagnostics()
	idIndex := document.NewIndex()
	for _, filePath := range docPaths {
		old := byPath[absPathOrSelf(filePath)]
		if old != nil && (keep[absPathOrSelf(filePath)] || unchangedOnDisk(old, filePath)) {
			admitDocument(filePath, old, nil, idIndex, diags, next)
			continue
		}
		tk, loadErr := s.loadTikiFile(filePath, nil, nil)
		if loadErr == nil && old != nil {
			carryForwardGitMeta(tk, old)
		}
		admitDocument(filePath, tk, loadErr, idIndex, diags, next)
	}

	res := diffTikis(s.tikis, next)
	res.Reconciled = true
	s.tikis = next
	s.diagnostics = diags
	// deferrals only matter for paths still waiting on a retry
	for path := range s.deferred {
		if !keep[path] {
			delete(s.deferred, path)
		}
	}
	slog.Info("tikis reconciled with disk",
		"added", len(res.Added), "updated", len(res.Updated), "removed", len(res.Removed),
		"rejections", len(diags.Rejections()))
	return res, nil
}

// diffTikis compares two id→tiki maps by pointer identity.
func diffTikis(prev, next map[string]*tiki.Tiki) ReloadResult {
	var res ReloadResult
	for id, tk := range next {
		old, existed := prev[id]
		switch {
		case !existed:
			res.Added = append(res.Added, id)
		case old != tk:
			res.Updated = append(res.Updated, id)
		}
	}
	for id := range prev {
		if _, still := next[id]; !still {
			res.Removed = append(res.Removed, id)
		}
	}
	sort.Strings(res.Added)
	sort.Strings(res.Updated)
	sort.Strings(res.Removed)
	res.Changed = len(res.Added)+len(res.Updated)+len(res.Removed) > 0
	return res
}

// carryForwardGitMeta copies the git-derived metadata of the tiki previously
// loaded from the same path onto a freshly parsed one. Git history does not
// change when a working-tree file is edited, so this matches what a full
// load would compute, minus the git calls.
func carryForwardGitMeta(loaded, old *tiki.Tiki) {
	loaded.SetCreatedAt(old.CreatedAt())
	if v, ok := old.Get("createdBy"); ok {
		loaded.Set("createdBy", v)
	} else {
		loaded.Delete("createdBy")
	}
	// UpdatedAt = max(file mtime, last commit); old.UpdatedAt already folds
	// in the last commit time.
	if old.UpdatedAt().After(loaded.UpdatedAt()) {
		loaded.SetUpdatedAt(old.UpdatedAt())
	}
}

// sameFileState reports whether info matches the file state tk was loaded
// from (or last saved as).
func sameFileState(tk *tiki.Tiki, info os.FileInfo) bool {
	return info.ModTime().Equal(tk.LoadedMtime) && info.Size() == tk.LoadedSize
}

func unchangedOnDisk(tk *tiki.Tiki, path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && sameFileState(tk, info)
}

// tikisByPathLocked indexes the loaded tikis by file path. Caller must hold
// s.mu (read or write).
func (s *TikiStore) tikisByPathLocked() map[string]*tiki.Tiki {
	byPath := make(map[string]*tiki.Tiki, len(s.tikis))
	for _, tk := range s.tikis {
		if tk.Path() != "" {
			byPath[tk.Path()] = tk
		}
	}
	return byPath
}

// deferOnce records a first sighting of a suspicious file and reports true;
// on the second sighting it clears the record and reports false. Caller must
// hold s.mu.
func (s *TikiStore) deferOnce(path string) bool {
	if s.deferred[path] {
		delete(s.deferred, path)
		return false
	}
	if s.deferred == nil {
		s.deferred = map[string]bool{}
	}
	s.deferred[path] = true
	return true
}

// clearDeferral forgets a pending retry for path. Caller must hold s.mu.
func (s *TikiStore) clearDeferral(path string) {
	delete(s.deferred, path)
}

// normalizePaths returns the paths in absolute, cleaned form, deduplicated
// and sorted, so tiki paths (always absolute) compare equal.
func normalizePaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		abs := absPathOrSelf(p)
		if seen[abs] {
			continue
		}
		seen[abs] = true
		out = append(out, abs)
	}
	sort.Strings(out)
	return out
}

// absPathOrSelf returns the absolute form of path, or path itself when it
// cannot be resolved.
func absPathOrSelf(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return filepath.Clean(path)
}
