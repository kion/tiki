package tikistore

import "time"

// parseStoredCreatedAt reads a frontmatter createdAt value.
// yaml.v3 turns an unquoted timestamp into a time.Time, so a quoted one is the only string case.
func parseStoredCreatedAt(v interface{}) (time.Time, bool) {
	switch val := v.(type) {
	case time.Time:
		if val.IsZero() {
			return time.Time{}, false
		}
		return val.UTC(), true
	case string:
		t, err := time.Parse(time.RFC3339Nano, val)
		if err != nil {
			return time.Time{}, false
		}
		return t.UTC(), true
	}
	return time.Time{}, false
}
