//go:build !cerebrai_dev

package telemetry

import "log/slog"

// ChatContentLogging reports whether this build logs raw conversation
// content. False in every normal build; see chatlog_dev.go. The desktop
// app's Preferences window surfaces it as a warning when true.
const ChatContentLogging = false

// LogChatExchange records a completed chat round-trip.
//
// Metadata only. Conversation content is the user's private memory store and
// inbox, and in OTLP mode log records are shipped off the machine to a
// collector, so raw content must not reach telemetry in a build anyone might
// actually run. Developers who need the full text can build with the
// cerebrai_dev tag (make run-desktop); that variant lives in chatlog_dev.go
// and is not compiled into a release binary.
func LogChatExchange(input, response string) {
	slog.Debug("chat exchange",
		"input_len", len(input),
		"response_len", len(response))
}
