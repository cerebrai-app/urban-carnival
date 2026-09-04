//go:build cerebrai_dev

package desktopui

import "log/slog"

// chatContentLogging reports whether this build logs raw conversation
// content. True here, which the Preferences window surfaces as a warning so
// a dev build is never mistaken for a normal one.
const chatContentLogging = true

// logChatExchange records a completed chat round-trip, including the raw
// text of both sides.
//
// This variant is compiled only under the cerebrai_dev build tag, so raw
// conversation content cannot reach telemetry in a distributed binary. It is
// still gated a second time by the log level: nothing is emitted unless the
// Preferences log level is set to debug.
//
// Do not build release artifacts with this tag. In OTLP mode these records
// leave the machine for whatever collector OTEL_EXPORTER_OTLP_ENDPOINT names.
func logChatExchange(input, response string) {
	slog.Debug("chat exchange",
		"input_len", len(input),
		"response_len", len(response),
		"input", input,
		"response", response)
}
