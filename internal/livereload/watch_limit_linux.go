package livereload

import (
	"errors"
	"fmt"
	"syscall"
)

// inotify reports its watch limit as ENOSPC,
// which reads as "no space left on device".
func explainWatchErr(err error) error {
	if errors.Is(err, syscall.ENOSPC) {
		return fmt.Errorf("inotify watch limit reached (raise fs.inotify.max_user_watches): %w", err)
	}
	return err
}
