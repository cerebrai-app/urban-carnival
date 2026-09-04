# cerebrai

cerebrai is a personal automation system and "second brain," described in
full in [DESIGN.md](DESIGN.md). Per that design, the product surface is a
native macOS desktop app talking to a background worker that owns
automation execution, scheduling, and memory; the CLI in this repo is a
debugging tool, not the primary interface.

## Layout

- `cmd/cerebrai/` — CLI entrypoint (debugging tool, see DESIGN.md §3)
- `cmd/cerebrai-desktop/` — desktop app entrypoint
- `internal/cli/` — cobra command tree
- `internal/desktopui/` — native desktop UI (chat + automation management),
  built with [Fyne](https://fyne.io)
- `internal/workerclient/` — client interface to the background worker's
  local API, used by the desktop UI; `Mock` stands in until the worker's
  IPC transport exists
- `internal/telemetry/` — OpenTelemetry (traces + metrics) setup
- `internal/version/` — build metadata injected via `-ldflags`

The background worker itself (schedule/trigger evaluation, automation
execution, memory store, LLM orchestration via Eino) is not yet scaffolded.

## Develop

```sh
make build         # build ./bin/cerebrai
make run           # go run the CLI
make build-desktop # build ./bin/cerebrai-desktop
make run-desktop   # go run the desktop app (against a mock worker client)
make test          # go test ./...
make vet           # go vet ./...
make lint          # golangci-lint run (requires golangci-lint installed locally)
make fmt           # gofmt -w .
```

## Telemetry

By default, spans and metrics are printed to stderr via the OpenTelemetry
stdout exporters — no collector required, no network calls.

Pass `--otlp` to export via OTLP/gRPC instead. By default (no
`OTEL_EXPORTER_OTLP_ENDPOINT` set) that looks for a collector at
`localhost:4317` over an insecure connection; if unreachable, telemetry
export fails silently after a short bounded timeout and never fails the
command itself.

To point at a different collector/backend, combine `--otlp` with the
standard OpenTelemetry environment variables, e.g.:

```sh
export OTEL_EXPORTER_OTLP_ENDPOINT=https://collector.example.com:4317
export OTEL_EXPORTER_OTLP_HEADERS="api-key=..."
cerebrai version --otlp
```

## Docker

```sh
docker build -t cerebrai .
docker run --rm cerebrai version
```
