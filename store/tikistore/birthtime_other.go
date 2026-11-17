//go:build !linux && !darwin && !windows

package tikistore

import (
	"os"
	"time"
)

func fileBirthTime(string, os.FileInfo) (time.Time, bool) {
	return time.Time{}, false
}
