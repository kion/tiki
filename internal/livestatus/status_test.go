package livestatus

import (
	"strings"
	"testing"
)

func TestValidateAgent(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
	}{
		{in: "✻ Claude"},
		{in: ""},
		{in: strings.Repeat("a", MaxAgentRunes)},
		{in: strings.Repeat("a", MaxAgentRunes+1), wantErr: true},
		{in: "two\nlines", wantErr: true},
	}
	for _, tt := range tests {
		if err := ValidateAgent(tt.in); (err != nil) != tt.wantErr {
			t.Errorf("ValidateAgent(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
		}
	}
}

func TestValidateMessage(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
	}{
		{in: "Implementing animation"},
		{in: strings.Repeat("é", MaxMessageRunes)},
		{in: strings.Repeat("é", MaxMessageRunes+1), wantErr: true},
		{in: "", wantErr: true},
		{in: "   ", wantErr: true},
		{in: "two\nlines", wantErr: true},
	}
	for _, tt := range tests {
		if err := ValidateMessage(tt.in); (err != nil) != tt.wantErr {
			t.Errorf("ValidateMessage(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
		}
	}
}
