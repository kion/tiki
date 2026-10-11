package tikidetail

import (
	"strings"
	"unicode"

	"github.com/boolean-maybe/ruki/idfmt"
	"github.com/boolean-maybe/tiki/store"
	tikipkg "github.com/boolean-maybe/tiki/tiki"
)

// renderKey identifies what a detail render depends on in the store: the
// tiki shown and every tiki it may reference (dependency lists, ids in the
// description). The store replaces a tiki object whenever it changes, so an
// unchanged key means a re-render would show the same content — and can be
// skipped, keeping scroll position and focus when unrelated tikis change.
type renderKey struct {
	shown *tikipkg.Tiki
	refs  map[string]*tikipkg.Tiki
}

func computeRenderKey(tk *tikipkg.Tiki, s store.Store) renderKey {
	key := renderKey{shown: tk}
	if tk == nil || s == nil {
		return key
	}
	key.refs = map[string]*tikipkg.Tiki{}
	for id := range referencedIDs(tk) {
		key.refs[id] = s.GetTiki(id) // nil when unresolved: resolving later changes the key
	}
	return key
}

func (k renderKey) equal(o renderKey) bool {
	if k.shown != o.shown || len(k.refs) != len(o.refs) {
		return false
	}
	for id, tk := range k.refs {
		if other, ok := o.refs[id]; !ok || other != tk {
			return false
		}
	}
	return true
}

// referencedIDs collects every token in the tiki's field values and body that
// is shaped like a document id. Over-inclusive on purpose: a false positive
// costs at most an extra re-render when that id appears in the store.
func referencedIDs(tk *tikipkg.Tiki) map[string]struct{} {
	ids := map[string]struct{}{}
	var collect func(v interface{})
	collect = func(v interface{}) {
		switch val := v.(type) {
		case string:
			addIDTokens(ids, val)
		case []string:
			for _, s := range val {
				addIDTokens(ids, s)
			}
		case []interface{}:
			for _, e := range val {
				collect(e)
			}
		}
	}
	for _, v := range tk.Fields {
		collect(v)
	}
	addIDTokens(ids, tk.Body())
	delete(ids, tk.ID())
	return ids
}

func addIDTokens(ids map[string]struct{}, text string) {
	for _, tok := range strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if upper := strings.ToUpper(tok); idfmt.IsValidID(upper) {
			ids[upper] = struct{}{}
		}
	}
}
