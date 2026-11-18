//go:build !windows

package idseq

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

func lockFile(path string, timeout time.Duration) (func(), error) {
	//nolint:gosec // G304: path is tiki's own state file
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open id sequence lock: %w", err)
	}
	deadline := time.Now().Add(timeout)
	for {
		err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return func() {
				_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
				_ = f.Close()
			}, nil
		}
		if err != unix.EWOULDBLOCK || time.Now().After(deadline) {
			_ = f.Close()
			return nil, fmt.Errorf("lock id sequence file %s: %w (another tiki may be holding it)", path, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
