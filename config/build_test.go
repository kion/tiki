package config

import (
	"runtime/debug"
	"testing"
)

func TestApplyBuildInfo(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		commit      string
		info        debug.BuildInfo
		wantVersion string
		wantCommit  string
	}{
		{
			name:        "go install at a tag",
			version:     "dev",
			commit:      "unknown",
			info:        debug.BuildInfo{Main: debug.Module{Version: "v0.6.1"}},
			wantVersion: "0.6.1",
			wantCommit:  "unknown",
		},
		{
			name:    "build from a git checkout",
			version: "dev",
			commit:  "unknown",
			info: debug.BuildInfo{
				Main:     debug.Module{Version: "v0.6.2-0.20261010120000-872cc5fabc12+dirty"},
				Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "872cc5fabc1234567890"}},
			},
			wantVersion: "0.6.2-0.20261010120000-872cc5fabc12+dirty",
			wantCommit:  "872cc5fabc1234567890",
		},
		{
			name:        "devel version is ignored",
			version:     "dev",
			commit:      "unknown",
			info:        debug.BuildInfo{Main: debug.Module{Version: "(devel)"}},
			wantVersion: "dev",
			wantCommit:  "unknown",
		},
		{
			name:    "ldflags values win",
			version: "0.6.1",
			commit:  "abc123",
			info: debug.BuildInfo{
				Main:     debug.Module{Version: "v9.9.9"},
				Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "def456"}},
			},
			wantVersion: "0.6.1",
			wantCommit:  "abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origVersion, origCommit := Version, GitCommit
			t.Cleanup(func() { Version, GitCommit = origVersion, origCommit })

			Version, GitCommit = tt.version, tt.commit
			applyBuildInfo(&tt.info)

			if Version != tt.wantVersion {
				t.Errorf("Version = %q, want %q", Version, tt.wantVersion)
			}
			if GitCommit != tt.wantCommit {
				t.Errorf("GitCommit = %q, want %q", GitCommit, tt.wantCommit)
			}
		})
	}
}
