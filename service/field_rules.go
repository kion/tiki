package service

import (
	"fmt"
	"reflect"
	"slices"
	"time"

	tikipkg "github.com/boolean-maybe/tiki/tiki"
	"github.com/boolean-maybe/tiki/workflow"
)

func fieldValue(tk *tikipkg.Tiki, name string) (interface{}, bool) {
	v, ok := tk.Get(name)
	if !ok || v == nil {
		return nil, false
	}
	switch val := v.(type) {
	case string:
		return val, val != ""
	case []string:
		return val, len(val) > 0
	case []interface{}:
		return val, len(val) > 0
	}
	return v, true
}

func sameValue(a, b interface{}) bool {
	if ta, ok := a.(time.Time); ok {
		if tb, ok := b.(time.Time); ok {
			return ta.Equal(tb)
		}
	}
	if la, ok := stringList(a); ok {
		if lb, ok := stringList(b); ok {
			return slices.Equal(la, lb)
		}
	}
	if reflect.DeepEqual(a, b) {
		return true
	}
	// a number can come back from YAML as another integer type
	return isNumber(a) && isNumber(b) && fmt.Sprint(a) == fmt.Sprint(b)
}

func stringList(v interface{}) ([]string, bool) {
	switch list := v.(type) {
	case []string:
		return list, true
	case []interface{}:
		out := make([]string, len(list))
		for i, e := range list {
			s, ok := e.(string)
			if !ok {
				return nil, false
			}
			out[i] = s
		}
		return out, true
	}
	return nil, false
}

func isNumber(v interface{}) bool {
	switch v.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return true
	}
	return false
}

func validateRequiredOnCreate(_, new *tikipkg.Tiki, _ []*tikipkg.Tiki) *Rejection {
	for _, f := range workflow.WorkflowFields() {
		if !f.Required {
			continue
		}
		if _, ok := fieldValue(new, f.Name); !ok {
			return &Rejection{Reason: fmt.Sprintf("%s is required", f.Name)}
		}
	}
	return nil
}

// validateRequiredOnUpdate only refuses clearing a value:
// a tiki written before the field became required stays editable.
func validateRequiredOnUpdate(old, new *tikipkg.Tiki, _ []*tikipkg.Tiki) *Rejection {
	for _, f := range workflow.WorkflowFields() {
		if !f.Required {
			continue
		}
		_, had := fieldValue(old, f.Name)
		_, has := fieldValue(new, f.Name)
		if had && !has {
			return &Rejection{Reason: fmt.Sprintf("%s is required", f.Name)}
		}
	}
	return nil
}

// validateImmutableOnUpdate allows filling an empty immutable field once,
// for tikis written before the field became immutable,
// except a prefix field on a tiki whose id already carries its prefix:
// filling it would make the id disagree with the fields.
func validateImmutableOnUpdate(old, new *tikipkg.Tiki, _ []*tikipkg.Tiki) *Rejection {
	for _, f := range workflow.WorkflowFields() {
		if !f.Immutable {
			continue
		}
		oldV, had := fieldValue(old, f.Name)
		newV, has := fieldValue(new, f.Name)
		switch {
		case had && (!has || !sameValue(oldV, newV)):
			return &Rejection{Reason: fmt.Sprintf("%s is immutable", f.Name)}
		case !had && has && f.HasPrefixes() && workflow.MatchesIDScheme(old.ID()):
			return &Rejection{Reason: fmt.Sprintf("%s is immutable", f.Name)}
		}
	}
	return nil
}
