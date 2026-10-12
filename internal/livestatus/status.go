// Package livestatus holds the per-tiki status an agent publishes while it works:
// the sidecar contract, its formatting for cards, and the redraw timing.
package livestatus

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Card borders are narrow, so the agent stays short and the message stays a phrase.
const (
	MaxAgentRunes   = 16
	MaxMessageRunes = 60
)

type Status struct {
	Agent     string
	Message   string
	UpdatedAt time.Time
}

func ValidateAgent(s string) error {
	if hasControl(s) {
		return errors.New("agent: must be a single line")
	}
	if n := utf8.RuneCountInString(s); n > MaxAgentRunes {
		return fmt.Errorf("agent: %d characters, at most %d", n, MaxAgentRunes)
	}
	return nil
}

func ValidateMessage(s string) error {
	if strings.TrimSpace(s) == "" {
		return errors.New("message: required")
	}
	if hasControl(s) {
		return errors.New("message: must be a single line")
	}
	if n := utf8.RuneCountInString(s); n > MaxMessageRunes {
		return fmt.Errorf("message: %d characters, at most %d (aim for 1-5 words)", n, MaxMessageRunes)
	}
	return nil
}

func hasControl(s string) bool {
	return strings.IndexFunc(s, unicode.IsControl) >= 0
}
