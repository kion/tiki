package workflow

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/boolean-maybe/ruki/idfmt"
)

// DefaultPrefixSeparator joins id prefix segments and the number when a workflow names none.
const DefaultPrefixSeparator = "-"

// projectPrefix and prefixSeparator are guarded by workflowMu, like the field registry they belong to.
var (
	projectPrefix   string
	prefixSeparator = DefaultPrefixSeparator
)

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

// SetPrefixSeparator records what joins prefix segments and the number: "-", "_" or "".
func SetPrefixSeparator(sep string) {
	workflowMu.Lock()
	prefixSeparator = sep
	workflowMu.Unlock()
}

// PrefixSeparator returns what joins prefix segments and the number.
func PrefixSeparator() string {
	workflowMu.RLock()
	defer workflowMu.RUnlock()
	return prefixSeparator
}

// FormatID builds the id numbered n under prefix.
func FormatID(prefix string, n int) string {
	return prefix + PrefixSeparator() + strconv.Itoa(n)
}

// SplitSchemeID splits id at the workflow's separator.
func SplitSchemeID(id string) (prefix string, number int, ok bool) {
	return idfmt.SplitID(id, PrefixSeparator())
}

// MatchesIDScheme reports whether id is one the workflow's scheme can issue.
// Shape alone can't tell: without a separator BUG123 could be random or sequential.
func MatchesIDScheme(id string) bool {
	if !PrefixedIDsEnabled() {
		return false
	}
	prefix, _, ok := SplitSchemeID(id)
	return ok && IsKnownIDPrefix(prefix)
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
	composed, _ := composedPrefixes(project, PrefixSeparator(), PrefixFields())
	_, ok := composed[prefix]
	return ok
}

// composedPrefixes maps every prefix the scheme can compose to the values that compose it,
// and reports the first prefix two different combinations compose alike.
func composedPrefixes(project, sep string, defs []FieldDef) (map[string]string, string) {
	type combo struct{ segments, values []string }
	combos := []combo{{}}
	for _, d := range defs {
		if !d.HasPrefixes() {
			continue
		}
		var next []combo
		for _, c := range combos {
			next = append(next, c)
			for _, v := range d.EnumValues {
				if v.Prefix == "" {
					continue
				}
				next = append(next, combo{
					segments: append(append([]string(nil), c.segments...), v.Prefix),
					values:   append(append([]string(nil), c.values...), d.Name+"="+v.Value),
				})
			}
		}
		combos = next
	}
	out := make(map[string]string, len(combos))
	clash := ""
	for _, c := range combos {
		prefix, what := project, "the top-level prefix"
		if len(c.segments) > 0 {
			prefix, what = strings.Join(c.segments, sep), strings.Join(c.values, " + ")
		}
		if prev, seen := out[prefix]; seen && prev != what && clash == "" {
			clash = fmt.Sprintf("%s and %s both compose %q", prev, what, prefix)
		}
		if _, seen := out[prefix]; !seen {
			out[prefix] = what
		}
	}
	return out, clash
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
	return strings.Join(segments, PrefixSeparator())
}

// ValidateIDScheme checks the top-level prefix, the separator and the value prefixes of defs.
// Value prefixes are unique across fields and differ from the top-level one,
// so every composed prefix maps back to exactly one combination of values.
func ValidateIDScheme(prefix, sep string, defs []FieldDef) error {
	if !idfmt.IsValidSeparator(sep) {
		return fmt.Errorf(`prefix separator %q isn't allowed: use "-", "_" or "" (none)`, sep)
	}
	if prefix != "" && !idfmt.IsValidPrefixSegment(prefix) {
		return fmt.Errorf("top-level prefix %q must be uppercase letters and digits, starting with a letter", prefix)
	}
	// without a separator the number is the trailing digits, so a prefix ending in one would blur it
	if sep == "" && prefix != "" && !endsInLetter(prefix) {
		return fmt.Errorf("top-level prefix %q must end in a letter when prefixSeparator is empty", prefix)
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
			if sep == "" && !endsInLetter(v.Prefix) {
				return fmt.Errorf("prefix %q (%s) must end in a letter when prefixSeparator is empty", v.Prefix, where)
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
	if longest := longestComposedPrefix(prefix, sep, defs); longest > maxComposedPrefix {
		return fmt.Errorf("value prefixes can combine into a %d-character prefix, which is too long: ids are at most %d characters including the separator and the number", longest, idfmt.MaxIDLength)
	}
	if prefix != "" {
		if _, clash := composedPrefixes(prefix, sep, defs); clash != "" {
			return fmt.Errorf("prefixes are ambiguous: %s; use a separator or different prefixes", clash)
		}
	}
	return nil
}

func endsInLetter(s string) bool {
	last := s[len(s)-1]
	return last >= 'A' && last <= 'Z'
}

// maxComposedPrefix leaves room for "-" and a number of the largest size idfmt accepts.
const maxComposedPrefix = idfmt.MaxIDLength - 10

func longestComposedPrefix(prefix, sep string, defs []FieldDef) int {
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
		total += (segments - 1) * len(sep)
	}
	return max(total, len(prefix))
}
