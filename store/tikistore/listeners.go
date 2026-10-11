package tikistore

import "github.com/boolean-maybe/tiki/store"

// AddListener registers a callback for change notifications.
// returns a listener ID that can be used to remove the listener.
func (s *TikiStore) AddListener(listener store.ChangeListener) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextListenerID
	s.nextListenerID++
	s.listeners[id] = listener
	return id
}

// RemoveListener removes a previously registered listener by ID
func (s *TikiStore) RemoveListener(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.listeners, id)
}

// BatchNotifications runs fn and coalesces the change notifications it
// causes into one, delivered when the outermost batch ends (none when nothing
// changed). Listeners re-render views, so applying many changes in a batch
// costs one re-render instead of one per change.
func (s *TikiStore) BatchNotifications(fn func()) {
	s.mu.Lock()
	s.batchDepth++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.batchDepth--
		flush := s.batchDepth == 0 && s.batchDirty
		if flush {
			s.batchDirty = false
		}
		s.mu.Unlock()
		if flush {
			s.notifyListeners()
		}
	}()
	fn()
}

func (s *TikiStore) notifyListeners() {
	s.mu.Lock()
	if s.batchDepth > 0 {
		s.batchDirty = true
		s.mu.Unlock()
		return
	}
	listeners := make([]store.ChangeListener, 0, len(s.listeners))
	for _, l := range s.listeners {
		listeners = append(listeners, l)
	}
	s.mu.Unlock()

	for _, l := range listeners {
		l()
	}
}
