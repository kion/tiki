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

type ReloadResult struct {
	Changed bool
	Added   []string
	Updated []string
	Removed []string
	// Retry holds known documents that read as empty or unparsable,
	// likely caught mid-write;
	// offer them again later (the second sighting is final).
	Retry      []string
	Reconciled bool
}

// ReloadPaths never calls git:
// Reload's history scans take seconds on large repos.
// Whatever the full-load rules decide (id changes, collisions, rejected files)
// falls back to Reconcile, so the result matches a fresh load.
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

// Reconcile is a git-free Reload, for when the changed paths are unknown
// (lost events, ignore-file changes, catching up after enabling live reload).
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

// ExternallyChanged lets live reload drop the echo of tiki's own writes
// before it reaches the UI goroutine.
// The read lock makes it wait for an in-progress save to finish stamping LoadedMtime.
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

type pathUpsert struct {
	path   string
	old    *tiki.Tiki
	loaded *tiki.Tiki
}

func (s *TikiStore) reloadPathsLocked(scope *document.Scope, paths []string) (ReloadResult, error) {
	var res ReloadResult
	byPath := s.tikisByPathLocked()

	var removals []*tiki.Tiki
	var upserts []pathUpsert
	keep := map[string]bool{} // mid-write files a fallback Reconcile must not re-read
	needReconcile := false

	for _, path := range normalizePaths(paths) {
		old := byPath[path]
		prevReason, rejected := s.diagnostics.reasonFor(path)
		if rejected && prevReason == LoadReasonDuplicateID {
			needReconcile = true // which file wins depends on the others
			continue
		}
		// safe to clear early: a fallback Reconcile rebuilds all rejections anyway
		dropRejection := func() {
			if rejected {
				s.diagnostics.clearPath(path)
			}
		}

		if !scope.IncludeFile(path) {
			if old != nil {
				removals = append(removals, old)
			}
			dropRejection()
			continue
		}

		info, statErr := os.Stat(path)
		if statErr != nil || info.IsDir() {
			if old != nil {
				removals = append(removals, old)
			}
			dropRejection()
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
				// plain wiki content: dropped silently, as in a full load
				if old != nil {
					removals = append(removals, old)
				}
				dropRejection()
				s.clearDeferral(path)
				continue
			}
			// likely mid-write (os.WriteFile truncates first):
			// give the writer one more batch before treating the failure as real
			if old != nil && s.deferOnce(path) {
				res.Retry = append(res.Retry, path)
				keep[path] = true
				continue
			}
			if old != nil {
				removals = append(removals, old)
			}
			if errors.Is(loadErr, errSkipNoID) {
				dropRejection() // an empty file is skipped silently, as in a full load
				continue
			}
			reason := rejectionReason(loadErr)
			logRejection(s.diagnostics, path, reason, "failed to load tiki file", "file", path, "reason", reason, "error", loadErr)
			s.diagnostics.setPath(path, reason, loadErr.Error())
			continue
		}
		s.clearDeferral(path)

		if old != nil && old.ID() != loaded.ID() {
			needReconcile = true
			continue
		}
		dropRejection()
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
			// a rename is an update to listeners, not a remove + add
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

func (s *TikiStore) needsReconcileForIDs(removals []*tiki.Tiki, upserts []pathUpsert) bool {
	// a rejected duplicate may be waiting to take over a freed id
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

// Unchanged files keep their tiki objects so views can tell nothing changed.
// Caller must hold s.mu.
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
			admitDocument(filePath, old, nil, idIndex, diags, s.diagnostics, next)
			continue
		}
		tk, loadErr := s.loadTikiFile(filePath, nil, nil)
		if loadErr == nil && old != nil {
			carryForwardGitMeta(tk, old)
		}
		admitDocument(filePath, tk, loadErr, idIndex, diags, s.diagnostics, next)
	}

	res := diffTikis(s.tikis, next)
	res.Reconciled = true
	s.tikis = next
	s.diagnostics = diags
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

// Editing a working-tree file doesn't change its git history,
// so the old values are what a full load would compute.
func carryForwardGitMeta(loaded, old *tiki.Tiki) {
	loaded.SetCreatedAt(old.CreatedAt())
	if v, ok := old.Get("createdBy"); ok {
		loaded.Set("createdBy", v)
	} else {
		loaded.Delete("createdBy")
	}
	// old.UpdatedAt already includes the last commit time
	if old.UpdatedAt().After(loaded.UpdatedAt()) {
		loaded.SetUpdatedAt(old.UpdatedAt())
	}
}

func sameFileState(tk *tiki.Tiki, info os.FileInfo) bool {
	return info.ModTime().Equal(tk.LoadedMtime) && info.Size() == tk.LoadedSize
}

func unchangedOnDisk(tk *tiki.Tiki, path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && sameFileState(tk, info)
}

// Caller must hold s.mu.
func (s *TikiStore) tikisByPathLocked() map[string]*tiki.Tiki {
	byPath := make(map[string]*tiki.Tiki, len(s.tikis))
	for _, tk := range s.tikis {
		if tk.Path() != "" {
			byPath[tk.Path()] = tk
		}
	}
	return byPath
}

// Caller must hold s.mu.
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

// Caller must hold s.mu.
func (s *TikiStore) clearDeferral(path string) {
	delete(s.deferred, path)
}

// Tiki paths are absolute; event paths must match them exactly.
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

func absPathOrSelf(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return filepath.Clean(path)
}
