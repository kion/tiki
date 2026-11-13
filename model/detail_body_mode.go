package model

import "sync"

// DetailBodyMode holds whether detail views show a tiki body rendered or as source.
// One instance is shared by every detail view,
// so the last used mode carries from tiki to tiki.
type DetailBodyMode struct {
	mu       sync.RWMutex
	source   bool
	onChange func(source bool)
}

// NewDetailBodyMode creates the mode.
// onChange, if set, runs after every toggle with the new value.
func NewDetailBodyMode(source bool, onChange func(source bool)) *DetailBodyMode {
	return &DetailBodyMode{source: source, onChange: onChange}
}

// Source reports whether bodies are shown as source.
func (m *DetailBodyMode) Source() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.source
}

// Toggle flips the mode and returns the new value.
func (m *DetailBodyMode) Toggle() bool {
	m.mu.Lock()
	m.source = !m.source
	source := m.source
	m.mu.Unlock()

	if m.onChange != nil {
		m.onChange(source)
	}
	return source
}
