package bootstrap

import (
	"github.com/rivo/tview"

	"github.com/boolean-maybe/tiki/config"
	"github.com/boolean-maybe/tiki/controller"
	"github.com/boolean-maybe/tiki/internal/livereload"
	"github.com/boolean-maybe/tiki/internal/livestatus"
	"github.com/boolean-maybe/tiki/model"
	"github.com/boolean-maybe/tiki/store/tikistore"
)

const liveReloadStatKey = "LiveReload"

// after version, workflow, user and branch
const liveReloadStatPriority = 5

// Plain QueueUpdate, plus a redraw only when the store changed:
// no repaint for no-op batches.
func newLiveReload(application *tview.Application, store *tikistore.TikiStore,
	statusline *model.StatuslineConfig, router *controller.InputRouter, live *livestatus.Registry) *livereload.Service {
	opts := LiveReloadOptions(config.GetDocDir(), store, statusline, router, live)
	opts.Dispatch = func(fn func()) { application.QueueUpdate(fn) }
	opts.Redraw = func() { application.ForceDraw() } // safe inside a queued update
	return livereload.New(opts)
}

// Shared with the integration test harness, which supplies its own Dispatch.
func LiveReloadOptions(root string, store *tikistore.TikiStore,
	statusline *model.StatuslineConfig, router *controller.InputRouter, live *livestatus.Registry) livereload.Options {
	onState, onErr := liveReloadStatus(statusline)
	opts := livereload.Options{
		Root:          root,
		Target:        store,
		Busy:          router.IsModalInteractionActive,
		OnStateChange: onState,
		OnError:       onErr,
	}
	if live != nil {
		opts.OnStateChange = func(on bool) {
			onState(on)
			live.SetEnabled(on)
		}
		opts.Sidecar = &livereload.Sidecar{Dir: livestatus.Dir(root), OnChange: live.Rescan}
	}
	return opts
}

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
