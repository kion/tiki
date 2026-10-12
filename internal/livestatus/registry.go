package livestatus

import (
	"errors"
	"io/fs"
	"os"
	"time"
)

type RegistryOptions struct {
	Root       string
	StaleAfter time.Duration
	Now        func() time.Time
	Animator   *Animator
	Warn       func(Problem)
}

// Registry is owned by the UI goroutine: cards read it while drawing.
type Registry struct {
	opts     RegistryOptions
	enabled  bool
	statuses map[string]Status
	warned   map[string]time.Time
}

func NewRegistry(opts RegistryOptions) *Registry {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Registry{opts: opts, warned: map[string]time.Time{}}
}

// SetEnabled follows live reload:
// with it off, tiki can't see updates, so a status would be a stale snapshot.
func (r *Registry) SetEnabled(on bool) {
	r.enabled = on
	if on {
		r.Rescan()
		return
	}
	r.statuses = nil
	if r.opts.Animator != nil {
		r.opts.Animator.Stop()
	}
}

func (r *Registry) Rescan() {
	if !r.enabled {
		return
	}
	statuses, problems := Scan(r.opts.Root)
	r.statuses = statuses
	for _, p := range problems {
		// rescans follow every change in the directory, so warn once per version of a file
		if seen, ok := r.warned[p.Path]; ok && seen.Equal(p.ModTime) {
			continue
		}
		r.warned[p.Path] = p.ModTime
		if r.opts.Warn != nil {
			r.opts.Warn(p)
		}
	}
}

func (r *Registry) Line(id string) (Line, bool) {
	st, ok := r.statuses[id]
	if !r.enabled || !ok {
		return Line{}, false
	}
	return Compose(st, r.opts.Now(), r.opts.StaleAfter), true
}

func (r *Registry) Drawn(next time.Duration) {
	if r.opts.Animator != nil {
		r.opts.Animator.Drawn(next)
	}
}

// Clear rescans at once,
// so the card updates without waiting for the file watcher.
func (r *Registry) Clear(id string) (bool, error) {
	if _, err := os.Stat(Path(r.opts.Root, id)); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err := Remove(r.opts.Root, id); err != nil {
		return false, err
	}
	r.Rescan()
	return true, nil
}
