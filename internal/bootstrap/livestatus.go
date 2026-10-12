package bootstrap

import (
	"log/slog"
	"time"

	"github.com/rivo/tview"

	"github.com/boolean-maybe/tiki/config"
	"github.com/boolean-maybe/tiki/controller"
	"github.com/boolean-maybe/tiki/internal/livestatus"
)

func newLiveStatus(application *tview.Application) (*livestatus.Registry, *livestatus.Animator) {
	anim := livestatus.NewAnimator(
		func(fn func()) { application.QueueUpdateDraw(fn) },
		livestatus.AfterFunc,
		time.Now,
	)
	reg := livestatus.NewRegistry(livestatus.RegistryOptions{
		Root:       config.GetDocDir(),
		StaleAfter: config.GetLiveStatusStaleAfter(),
		Animator:   anim,
		Warn: func(p livestatus.Problem) {
			slog.Warn("live status: ignoring unreadable file", "path", p.Path, "error", p.Err)
		},
	})
	return reg, anim
}

// Shared with the integration test harness, which builds its own controllers.
func SetLiveStatusClearer(plugins map[string]controller.PluginControllerInterface, live controller.LiveStatusClearer) {
	for _, pc := range plugins {
		if c, ok := pc.(interface {
			SetLiveStatus(controller.LiveStatusClearer)
		}); ok {
			c.SetLiveStatus(live)
		}
	}
}
