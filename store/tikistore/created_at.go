package tikistore

import (
	"time"

	"github.com/boolean-maybe/tiki/tiki"
)

// yaml.v3's own timestamp layouts, so a quoted value is read the same as an unquoted one.
var storedCreatedAtLayouts = []string{
	"2006-1-2T15:4:5.999999999Z07:00",
	"2006-1-2t15:4:5.999999999Z07:00",
	"2006-1-2 15:4:5.999999999",
	"2006-1-2",
}

func parseStoredCreatedAt(v interface{}) (time.Time, bool) {
	switch val := v.(type) {
	case time.Time:
		if val.IsZero() {
			return time.Time{}, false
		}
		return val.UTC(), true
	case string:
		for _, layout := range storedCreatedAtLayouts {
			if t, err := time.Parse(layout, val); err == nil {
				return t.UTC(), true
			}
		}
	}
	return time.Time{}, false
}

// Memory holds exactly what lands in the file (UTC, whole seconds),
// so the value doesn't shift at the next load.
func writtenCreatedAt(t time.Time) time.Time {
	return t.UTC().Truncate(time.Second)
}

func createdAtLineValue(t time.Time) string {
	return writtenCreatedAt(t).Format(time.RFC3339)
}

// A guess stays out of the file: written once, it would win over git from then on.
func writesCreatedAt(t *tiki.Tiki) bool {
	return !t.CreatedAt().IsZero() && t.CreatedAtSource() != tiki.CreatedAtGuess
}
