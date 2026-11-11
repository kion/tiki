package tikistore

import (
	"sort"
	"time"

	"github.com/boolean-maybe/tiki/internal/livestatus"
	"github.com/boolean-maybe/tiki/tiki"
)

// Only the times are kept: they give ruki's activeAt.
// Unreadable files are skipped here; the board's registry is what reports them.
func scanLiveTimes(root string) map[string]time.Time {
	statuses, _ := livestatus.Scan(root)
	times := make(map[string]time.Time, len(statuses))
	for id, st := range statuses {
		times[id] = st.UpdatedAt
	}
	return times
}

// Tikis may already be handed out, so one whose time changes is replaced by a copy.
func applyLiveTimes(tikis map[string]*tiki.Tiki, live map[string]time.Time) []string {
	var changed []string
	for id, tk := range tikis {
		at := live[id]
		if tk.LiveAt().Equal(at) {
			continue
		}
		c := tk.Clone()
		c.SetLiveAt(at)
		tikis[id] = c
		changed = append(changed, id)
	}
	sort.Strings(changed)
	return changed
}

// ReloadLive follows a change in the live-status directory.
func (s *TikiStore) ReloadLive() ReloadResult {
	s.mu.Lock()
	s.live = scanLiveTimes(s.dir)
	changed := applyLiveTimes(s.tikis, s.live)
	s.mu.Unlock()

	res := ReloadResult{Updated: changed, Changed: len(changed) > 0}
	if res.Changed {
		s.notifyListeners()
	}
	return res
}
