// Package version holds build metadata injected at link time via -ldflags.
package version

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
