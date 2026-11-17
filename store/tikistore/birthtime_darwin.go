package tikistore

import (
	"os"
	"syscall"
	"time"
)

func fileBirthTime(_ string, info os.FileInfo) (time.Time, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(st.Birthtimespec.Unix()), true
}
