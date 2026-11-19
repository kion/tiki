package tikistore

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/boolean-maybe/ruki/idfmt"
	"github.com/boolean-maybe/tiki/config"
	"github.com/boolean-maybe/tiki/internal/idseq"
	"github.com/boolean-maybe/tiki/tiki"
	"github.com/boolean-maybe/tiki/workflow"
)

// IDAllocator hands out the numbers behind prefixed ids, one sequence per prefix.
type IDAllocator interface {
	Next(prefix string, floor int) (int, error)
	Raise(prefix string, atLeast int) error
	Peek(prefix string) (int, error)
}

// WithIDAllocator replaces the default counter file, for tests.
func WithIDAllocator(a IDAllocator) Option {
	return func(s *TikiStore) { s.idAlloc = a }
}

// allocatorLocked opens the default counter file on first use,
// so workflows without prefixed ids never touch the state dir.
func (s *TikiStore) allocatorLocked() (IDAllocator, error) {
	if s.idAlloc == nil {
		f, err := s.counterFile()
		if err != nil {
			return nil, err
		}
		s.idAlloc = f
	}
	return s.idAlloc, nil
}

// CounterFile reports this project's counter file and what it holds;
// exists is false until the first prefixed id or rename writes it.
func (s *TikiStore) CounterFile() (path string, counters map[string]int, exists bool, err error) {
	f, err := s.counterFile()
	if err != nil {
		return "", nil, false, err
	}
	if path, exists, err = f.Path(); err != nil {
		return "", nil, false, err
	}
	counters, err = f.Counters()
	return path, counters, exists, err
}

func (s *TikiStore) counterFile() (*idseq.File, error) {
	key, label, checkout, err := s.sequenceKey()
	if err != nil {
		return nil, err
	}
	state := config.GetStateDir()
	if state == "" {
		return nil, errors.New("no state directory for the id counter: set XDG_STATE_HOME or HOME")
	}
	return idseq.Open(filepath.Join(state, "sequences"), key, label, checkout), nil
}

// sequenceKey keys the counter file by the repository's root commit,
// which every clone, branch and worktree shares;
// a repository without commits falls back to its shared git dir,
// and a project without git to the scan root.
// Any other git failure is an error rather than a fallback:
// a silent fallback would give the repository a second counter.
// The label is the main checkout's folder name, which worktrees share.
func (s *TikiStore) sequenceKey() (key, label, checkout string, err error) {
	checkout = s.dir
	if abs, err := filepath.Abs(s.dir); err == nil {
		checkout = abs
	}
	if s.gitUtil != nil {
		if common, err := s.gitUtil.CommonDir(); err == nil && common != "" {
			label = filepath.Base(filepath.Dir(common))
			roots, err := s.gitUtil.RootCommits()
			if err != nil {
				return "", "", "", fmt.Errorf("can't key the id counter: %w", err)
			}
			if len(roots) > 0 {
				return "commit:" + roots[0], label, checkout, nil
			}
			return "gitdir:" + common, label, checkout, nil
		}
	}
	return "dir:" + checkout, filepath.Base(checkout), checkout, nil
}

// allocatePrefixedIDLocked gives tk the next id for the prefix its fields compose.
// The floor covers ids the counter file can't know about:
// tikis merged in from another machine, or a lost counter file.
func (s *TikiStore) allocatePrefixedIDLocked(tk *tiki.Tiki) (string, error) {
	prefix := workflow.IDPrefixFor(func(field string) string {
		v, _, _ := tk.StringField(field)
		return v
	})
	alloc, err := s.allocatorLocked()
	if err != nil {
		return "", err
	}
	for i := 0; i <= maxGenerateRetries; i++ {
		n, err := alloc.Next(prefix, s.highestNumberLocked(prefix))
		if err != nil {
			return "", fmt.Errorf("allocate id: %w", err)
		}
		id := workflow.FormatID(prefix, n)
		if !idfmt.IsValidID(id) {
			return "", fmt.Errorf("the next id would be %s, which is past the largest id tiki accepts", id)
		}
		if _, taken := s.tikis[id]; !taken {
			return id, nil
		}
	}
	return "", fmt.Errorf("failed to allocate a free %s id after %d attempts", prefix, maxGenerateRetries)
}

// highestNumberLocked counts former ids too: reissuing one would point old references at a new tiki.
func (s *TikiStore) highestNumberLocked(prefix string) int {
	highest := 0
	consider := func(id string) {
		if p, n, ok := workflow.SplitSchemeID(id); ok && p == prefix && n > highest {
			highest = n
		}
	}
	for id, tk := range s.tikis {
		consider(id)
		for _, former := range formerIDs(tk) {
			consider(former)
		}
	}
	return highest
}

// formerIDsField holds the ids a tiki had before `tiki ids rename|migrate`.
const formerIDsField = "formerIds"

func formerIDs(tk *tiki.Tiki) []string {
	v, ok := tk.Get(formerIDsField)
	if !ok {
		return nil
	}
	switch list := v.(type) {
	case []string:
		return list
	case []interface{}:
		out := make([]string, 0, len(list))
		for _, e := range list {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
