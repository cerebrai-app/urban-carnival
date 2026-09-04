//go:build !cerebrai_dev

package desktopui

import "log/slog"

// chatContentLogging reports whether this build logs raw conversation
// content. False in every normal build; see chatlog_dev.go.
const chatContentLogging = false

// logChatExchange records a completed chat round-trip.
//
// Metadata only. Conversation content is the user's private memory store and
// inbox, and in OTLP mode log records are shipped off the machine to a
// collector, so raw content must not reach telemetry in a build anyone might
// actually run. Developers who need the full text can build with the
// cerebrai_dev tag (make run-desktop); that variant lives in chatlog_dev.go
// and is not compiled into a release binary.
func logChatExchange(input, response string) {
	slog.Debug("chat exchange",
		"input_len", len(input),
		"response_len", len(response))
}
