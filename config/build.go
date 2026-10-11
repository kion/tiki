package config

import (
	"runtime/debug"
	"strings"
)

// Build information variables injected via ldflags at compile time.
// These are set by the build process (Makefile or GoReleaser) using:
// -ldflags "-X tiki/config.Version=... -X tiki/config.GitCommit=... -X tiki/config.BuildDate=..."
// When ldflags are absent (e.g. `go install`),
// Version and GitCommit fall back to the build info Go embeds in the binary.
var (
	// Version is the semantic version or commit hash.
	Version = "dev"

	// GitCommit is the full git commit hash.
	GitCommit = "unknown"

	// BuildDate is the ISO 8601 build timestamp.
	BuildDate = "unknown"
)

func init() {
	if info, ok := debug.ReadBuildInfo(); ok {
		applyBuildInfo(info)
	}
}

// The module version drops its leading "v" to match the Makefile and GoReleaser.
// BuildDate is left alone:
// build info only carries the commit time, not the build time.
func applyBuildInfo(info *debug.BuildInfo) {
	if Version == "dev" {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			Version = strings.TrimPrefix(v, "v")
		}
	}
	if GitCommit == "unknown" {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && s.Value != "" {
				GitCommit = s.Value
			}
		}
	}
}
