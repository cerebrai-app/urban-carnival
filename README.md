# cerebrai

cerebrai is a Go command-line tool.

## Layout

- `cmd/cerebrai/` — main package / entrypoint
- `internal/cli/` — cobra command tree
- `internal/telemetry/` — OpenTelemetry (traces + metrics) setup
- `internal/version/` — build metadata injected via `-ldflags`

## Develop

```sh
make build   # build ./bin/cerebrai
make run     # go run the CLI
make test    # go test ./...
make vet     # go vet ./...
make lint    # golangci-lint run (requires golangci-lint installed locally)
make fmt     # gofmt -w .
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
