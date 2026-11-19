package tikidetail

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/boolean-maybe/ruki/idfmt"
	"github.com/boolean-maybe/tiki/store"
	tikipkg "github.com/boolean-maybe/tiki/tiki"
)

// The store replaces a tiki object whenever it changes,
// so comparing pointers tells whether a re-render would show anything new.
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

// Over-inclusive on purpose: a false positive costs at most an extra re-render.
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

// prefixedIDPattern catches ids like MEM-DB-35 and MEM_DB_35,
// which the tokenizer below would split at every separator.
var prefixedIDPattern = regexp.MustCompile(`(?i)[a-z][a-z0-9]*(?:[-_][a-z][a-z0-9]*)*[-_][0-9]+`)

func addIDTokens(ids map[string]struct{}, text string) {
	for _, m := range prefixedIDPattern.FindAllString(text, -1) {
		if upper := strings.ToUpper(m); idfmt.IsValidID(upper) {
			ids[upper] = struct{}{}
		}
	}
	for _, tok := range strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if upper := strings.ToUpper(tok); idfmt.IsValidID(upper) {
			ids[upper] = struct{}{}
		}
	}
}
