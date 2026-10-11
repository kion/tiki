package bootstrap

import (
	"github.com/rivo/tview"

	"github.com/boolean-maybe/tiki/config"
	"github.com/boolean-maybe/tiki/controller"
	"github.com/boolean-maybe/tiki/internal/livereload"
	"github.com/boolean-maybe/tiki/model"
	"github.com/boolean-maybe/tiki/store/tikistore"
)

// liveReloadStatKey is the statusline left-stat key of the live indicator.
const liveReloadStatKey = "LiveReload"

// liveReloadStatPriority places the indicator after the version, workflow,
// user and branch stats.
const liveReloadStatPriority = 5

// newLiveReload builds the live reload service for the TUI. External changes
// are applied on the UI goroutine through QueueUpdate, and the screen is
// redrawn only when a batch changed the store.
func newLiveReload(application *tview.Application, store *tikistore.TikiStore,
	statusline *model.StatuslineConfig, router *controller.InputRouter) *livereload.Service {
	opts := LiveReloadOptions(config.GetDocDir(), store, statusline, router)
	opts.Dispatch = func(fn func()) { application.QueueUpdate(fn) }
	opts.Redraw = func() { application.ForceDraw() } // safe inside a queued update
	return livereload.New(opts)
}

// LiveReloadOptions returns the live reload wiring shared by the TUI and the
// integration test harness: the store as target, batches held while the
// router reports an overlay or input box open, and statusline reporting. The
// caller supplies Dispatch and Redraw for its UI loop.
func LiveReloadOptions(root string, store *tikistore.TikiStore,
	statusline *model.StatuslineConfig, router *controller.InputRouter) livereload.Options {
	onState, onErr := liveReloadStatus(statusline)
	return livereload.Options{
		Root:          root,
		Target:        store,
		Busy:          router.IsModalInteractionActive,
		OnStateChange: onState,
		OnError:       onErr,
	}
}

// liveReloadStatus returns the statusline sinks for the live reload service:
// a "● live" indicator while it is on, and error messages.
func liveReloadStatus(statusline *model.StatuslineConfig) (onState func(bool), onErr func(error)) {
	onState = func(enabled bool) {
		if enabled {
			statusline.SetLeftStat(liveReloadStatKey, "● live", liveReloadStatPriority)
			return
		}
		statusline.RemoveLeftStat(liveReloadStatKey)
	}
	onErr = func(err error) {
		statusline.SetMessage(err.Error(), model.MessageLevelError, true)
	}
	return onState, onErr
}
