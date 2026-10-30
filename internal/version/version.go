package version

import "fmt"

// Build-time variables injected via -ldflags
var (
	// Version is the current semantic version of Kerob.
	Version = "v0.1.1"

	// Commit is the git commit SHA of the build.
	Commit = "dev"

	// BuildDate is the RFC3339 timestamp of the build.
	BuildDate = "unknown"
)

// Info returns a formatted multi-line version and build summary.
func Info() string {
	return fmt.Sprintf("Kerob Automation Engine %s (commit: %s, built: %s)", Version, Commit, BuildDate)
}

// Short returns a concise version string.
func Short() string {
	return Version
}
