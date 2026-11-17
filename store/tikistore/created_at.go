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

// writtenCreatedAt is the value as it lands in a file: UTC, whole seconds.
// tiki keeps the same value in memory, so it doesn't shift at the next load.
func writtenCreatedAt(t time.Time) time.Time {
	return t.UTC().Truncate(time.Second)
}

func createdAtLineValue(t time.Time) string {
	return writtenCreatedAt(t).Format(time.RFC3339)
}
