package livereload

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
	"testing"
)

func TestExplainWatchErr_InotifyLimit(t *testing.T) {
	err := explainWatchErr(fmt.Errorf("watching /x: %w", syscall.ENOSPC))

	if !strings.Contains(err.Error(), "fs.inotify.max_user_watches") {
		t.Errorf("message %q doesn't say how to raise the limit", err)
	}
	if !errors.Is(err, syscall.ENOSPC) {
		t.Error("cause lost")
	}
}

func TestExplainWatchErr_OtherErrorsUnchanged(t *testing.T) {
	orig := fmt.Errorf("watching /x: %w", syscall.EACCES)
	if err := explainWatchErr(orig); err != orig {
		t.Errorf("unrelated error rewritten: %v", err)
	}
}
