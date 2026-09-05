// Package config holds cerebrai's build- and environment-derived
// configuration: version metadata injected at link time via -ldflags (see
// version.go) and the CEREBRAI_* environment variables that tune the desktop
// app (see env.go). The developer-checkout gate itself lives in
// internal/devmode.
package config

var (
	// Version is the semantic version of the build, set via -ldflags.
	Version = "dev"
	// Commit is the git commit SHA the build was produced from.
	Commit = "none"
	// Date is the build timestamp in RFC3339 format.
	Date = "unknown"
)

// String returns a human-readable build identifier.
func String() string {
	return Version + " (" + Commit + ", " + Date + ")"
}
