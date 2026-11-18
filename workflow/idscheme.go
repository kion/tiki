package workflow

import (
	"fmt"
	"strings"

	"github.com/boolean-maybe/ruki/idfmt"
)

// projectPrefix is guarded by workflowMu, like the field registry it belongs to.
var projectPrefix string

// HasPrefixes reports whether any of the field's enum values carries an id prefix.
func (f FieldDef) HasPrefixes() bool {
	for _, v := range f.EnumValues {
		if v.Prefix != "" {
			return true
		}
	}
	return false
}

// SetProjectPrefix records the workflow's top-level prefix; empty turns prefixed ids off.
func SetProjectPrefix(p string) {
	workflowMu.Lock()
	projectPrefix = p
	workflowMu.Unlock()
}

// ProjectPrefix returns the workflow's top-level prefix, or "" when prefixed ids are off.
func ProjectPrefix() string {
	workflowMu.RLock()
	defer workflowMu.RUnlock()
	return projectPrefix
}

// PrefixedIDsEnabled reports whether new tikis get prefixed sequential ids.
func PrefixedIDsEnabled() bool {
	return ProjectPrefix() != ""
}

// PrefixFields returns the registered workflow fields that carry value prefixes,
// in declaration order.
func PrefixFields() []FieldDef {
	var out []FieldDef
	for _, f := range WorkflowFields() {
		if f.HasPrefixes() {
			out = append(out, f)
		}
	}
	return out
}

// IsKnownIDPrefix reports whether prefix is one the workflow's scheme can compose.
func IsKnownIDPrefix(prefix string) bool {
	project := ProjectPrefix()
	if project == "" {
		return false
	}
	if prefix == project {
		return true
	}
	known := map[string]bool{}
	for _, f := range PrefixFields() {
		for _, v := range f.EnumValues {
			if v.Prefix != "" {
				known[v.Prefix] = true
			}
		}
	}
	for _, segment := range strings.Split(prefix, "-") {
		if !known[segment] {
			return false
		}
	}
	return true
}

// IDPrefixFor composes a tiki's id prefix from its prefix-field values,
// falling back to the project prefix when no value contributes a segment.
func IDPrefixFor(value func(field string) string) string {
	var segments []string
	for _, f := range PrefixFields() {
		v := value(f.Name)
		if v == "" {
			continue
		}
		if ev, ok := f.LookupEnum(v); ok && ev.Prefix != "" {
			segments = append(segments, ev.Prefix)
		}
	}
	if len(segments) == 0 {
		return ProjectPrefix()
	}
	return strings.Join(segments, "-")
}

// ValidateIDScheme checks the top-level prefix and the value prefixes of defs.
// Value prefixes are unique across fields and differ from the top-level one,
// so every composed prefix maps back to exactly one combination of values.
func ValidateIDScheme(prefix string, defs []FieldDef) error {
	if prefix != "" && !idfmt.IsValidPrefixSegment(prefix) {
		return fmt.Errorf("top-level prefix %q must be uppercase letters and digits, starting with a letter", prefix)
	}
	owner := make(map[string]string)
	for _, d := range defs {
		if !d.HasPrefixes() {
			continue
		}
		if prefix == "" {
			return fmt.Errorf("field %q has value prefixes, so the workflow needs a top-level prefix", d.Name)
		}
		if d.Type != TypeEnum || !d.Required || !d.Immutable {
			return fmt.Errorf("field %q has value prefixes, so it must be an enum with required: true and immutable: true", d.Name)
		}
		for _, v := range d.EnumValues {
			if v.Prefix == "" {
				continue
			}
			where := fmt.Sprintf("field %q, value %q", d.Name, v.Value)
			if !idfmt.IsValidPrefixSegment(v.Prefix) {
				return fmt.Errorf("prefix %q (%s) must be uppercase letters and digits, starting with a letter", v.Prefix, where)
			}
			if v.Prefix == prefix {
				return fmt.Errorf("prefix %q (%s) is the same as the top-level prefix", v.Prefix, where)
			}
			if prev, ok := owner[v.Prefix]; ok {
				return fmt.Errorf("prefix %q is used twice (%s and %s)", v.Prefix, prev, where)
			}
			owner[v.Prefix] = where
		}
	}
	if longest := longestComposedPrefix(prefix, defs); longest > maxComposedPrefix {
		return fmt.Errorf("value prefixes can combine into a %d-character prefix, which is too long: ids are at most %d characters including \"-\" and the number", longest, idfmt.MaxIDLength)
	}
	return nil
}

// maxComposedPrefix leaves room for "-" and a number of the largest size idfmt accepts.
const maxComposedPrefix = idfmt.MaxIDLength - 10

func longestComposedPrefix(prefix string, defs []FieldDef) int {
	total, segments := 0, 0
	for _, d := range defs {
		longest := 0
		for _, v := range d.EnumValues {
			longest = max(longest, len(v.Prefix))
		}
		if longest > 0 {
			total += longest
			segments++
		}
	}
	if segments > 1 {
		total += segments - 1
	}
	return max(total, len(prefix))
}
