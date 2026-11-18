//go:build windows

package idseq

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

func lockFile(path string, timeout time.Duration) (func(), error) {
	//nolint:gosec // G304: path is tiki's own state file
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open id sequence lock: %w", err)
	}
	h := windows.Handle(f.Fd())
	deadline := time.Now().Add(timeout)
	for {
		ol := new(windows.Overlapped)
		err = windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
		if err == nil {
			return func() {
				_ = windows.UnlockFileEx(h, 0, 1, 0, new(windows.Overlapped))
				_ = f.Close()
			}, nil
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, fmt.Errorf("lock id sequence file %s: %w (another tiki may be holding it)", path, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
