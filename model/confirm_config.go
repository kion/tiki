package model

import "sync"

// ConfirmConfig manages the visibility and pending action of the confirmation dialog.
type ConfirmConfig struct {
	mu sync.RWMutex

	visible   bool
	onConfirm func()

	listeners    map[int]func()
	nextListener int
}

// NewConfirmConfig creates a new config (hidden by default).
func NewConfirmConfig() *ConfirmConfig {
	return &ConfirmConfig{
		listeners:    make(map[int]func()),
		nextListener: 1,
	}
}

func (cc *ConfirmConfig) IsVisible() bool {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return cc.visible
}

func (cc *ConfirmConfig) SetVisible(visible bool) {
	cc.mu.Lock()
	changed := cc.visible != visible
	cc.visible = visible
	cc.mu.Unlock()
	if changed {
		cc.notifyListeners()
	}
}

func (cc *ConfirmConfig) SetOnConfirm(fn func()) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.onConfirm = fn
}

// Confirm hides the dialog before running the action,
// so focus is back on the view before a delete can navigate away from it.
func (cc *ConfirmConfig) Confirm() {
	cc.mu.RLock()
	fn := cc.onConfirm
	cc.mu.RUnlock()
	cc.SetVisible(false)
	if fn != nil {
		fn()
	}
}

func (cc *ConfirmConfig) Cancel() {
	cc.SetVisible(false)
}

func (cc *ConfirmConfig) AddListener(listener func()) int {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	id := cc.nextListener
	cc.nextListener++
	cc.listeners[id] = listener
	return id
}

func (cc *ConfirmConfig) RemoveListener(id int) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	delete(cc.listeners, id)
}

func (cc *ConfirmConfig) notifyListeners() {
	cc.mu.RLock()
	listeners := make([]func(), 0, len(cc.listeners))
	for _, l := range cc.listeners {
		listeners = append(listeners, l)
	}
	cc.mu.RUnlock()

	for _, l := range listeners {
		l()
	}
}
