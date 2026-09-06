# cerebrai

<img src="build/macos/icon.png" alt="CerebrAI app icon" width="120" align="right">

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
  local API, used by the desktop UI; the SQLite-backed `SQLite` stands in
  until the worker's IPC transport exists
- `internal/storage/` — opens cerebrai's SQLite database and keeps its
  schema up to date; see [Data storage](#data-storage)
- `internal/telemetry/` — OpenTelemetry (traces + metrics) setup, plus
  `LogChatExchange` whose payload is `cerebrai_dev`-gated (see
  [Chat content logging](#chat-content-logging))
- `internal/config/` — build metadata injected via `-ldflags`, and the
  `EnvLogLevel` / `EnvDBPath` env var names (`EnvLogLevel` used by
  `internal/desktopui`; `EnvDBPath` by `internal/storage`)
- `internal/devmode/` — everything active only in a developer's checkout:
  the `CEREBRAI_DEV_MODE` gate (`devmode.Enabled`, used by
  `internal/desktopui` and `internal/storage`) and the local Claude Code
  model (`devmode/claudecode`)

The background worker itself (schedule/trigger evaluation, automation
execution, memory store, LLM orchestration via Eino) is not yet scaffolded.

## Data storage

The desktop app persists its data (automations, and chat sessions with
their message history) in a SQLite database, opened via
`internal/storage.Open`. Where that database lives
depends on `CEREBRAI_DEV_MODE` (see [Configuration](#configuration)),
the same flag that reveals the Developer preferences section:

- **`CEREBRAI_DEV_MODE` set** (`make run-desktop` sets it for you):
  `./cerebrai.db` at the repo root. It's gitignored — inspect it with
  `sqlite3 cerebrai.db`, or delete it to start fresh.
- **Unset** (the default — `make build-desktop`, or a packaged release): the
  OS's per-user application data directory, e.g.
  `~/Library/Application Support/cerebrai/cerebrai.db` on macOS.

`CEREBRAI_DB_PATH`, if set, overrides both locations with a literal path.
`make install-macos` sets it (to the checkout's `cerebrai.db`) so the dev
build it installs still finds a database when launched from Finder, where
the working directory is `/` and a bare `./cerebrai.db` is unwritable.

Until the background worker exists, `cmd/cerebrai-desktop` opens this
database directly and uses `workerclient.SQLite` as its `Client`, so
automations and chat history survive restarts.

## Develop

```sh
make build         # build ./bin/cerebrai
make run           # go run the CLI
make build-desktop # build ./bin/cerebrai-desktop
make run-desktop   # go run the desktop app (dev build, SQLite-backed, see Data storage)
make test          # go test ./...
make vet           # go vet ./... (both the default and dev build variants)
make lint          # golangci-lint run (requires golangci-lint installed locally)
make fmt           # gofmt -w .

make package-macos # wrap bin/cerebrai-desktop in dist/macos/CerebrAI.app (add DMG=1 for an installer)
make install-macos # build + overwrite the CerebrAI.app in ~/Applications, as a dev build (see below)
```

macOS packaging details are in [build/macos/README.md](build/macos/README.md).

## Configuration

The desktop app reads these environment variables at startup:

| Variable | Values | Default | Effect |
| --- | --- | --- | --- |
| `CEREBRAI_DEV_MODE` | `1` / `true` / `0` / `false` | unset (off) | Shows the **Developer** section of the Preferences window (the OTLP export toggle and dev-build status), and stores the SQLite database in the repo root instead of the OS's per-user application data directory (see [Data storage](#data-storage)). `make run-desktop` sets this for you. |
| `CEREBRAI_LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` | Minimum log level. Not settable or shown in the UI. An unrecognized value warns on stderr and falls back to the default. |
| `CEREBRAI_DB_PATH` | a filesystem path | unset | Overrides where the SQLite database lives, ahead of the `CEREBRAI_DEV_MODE` and per-user-app-data defaults (see [Data storage](#data-storage)). `make install-macos` sets it. |
| `OTEL_EXPORTER_OTLP_*` | see [Telemetry](#telemetry) | — | Standard OpenTelemetry exporter configuration. |

The `CEREBRAI_*` variables are read once at startup, so changing one means
restarting the app. They are developer controls, deliberately kept out of
the persisted user preferences.

Put them in a `.env` file in the repo root for local development. It is
gitignored, and the `Makefile` exports the names it defines to every target,
so `make run-desktop` picks them up:

```sh
# .env
CEREBRAI_DEV_MODE=1
CEREBRAI_LOG_LEVEL=debug
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4317
```

Nothing loads `.env` at runtime — a packaged app is configured by its actual
environment, not by a file it happens to find next to itself. The one bridge
is `make install-macos`: it builds with the `cerebrai_dev` tag and copies the
`CEREBRAI_*` and `OTEL_*` names from `.env` into the installed bundle's
`Info.plist` as `LSEnvironment`, so the app you install matches
`make run-desktop` (plus `CEREBRAI_DB_PATH` pinned to the checkout — see
[Data storage](#data-storage)). Plain `make package-macos` does neither; its
bundle stays a clean release build.

The CLI takes `--log-level` and `--print-telemetry` (see
[Telemetry](#telemetry)).

## Telemetry

The CLI exports spans and metrics via OTLP/gRPC whenever an OTLP collector
endpoint is configured — `OTEL_EXPORTER_OTLP_ENDPOINT`, which a developer's
checkout sets in `.env`. If that collector is unreachable the export fails
silently after a short bounded timeout and never fails the command itself.

With no endpoint configured, spans and metrics go nowhere — an installed CLI
stays quiet — unless you pass `--print-telemetry`, which prints them to
stderr via the OpenTelemetry stdout exporters (no collector, no network).
Logs go to stderr either way; `--log-level` sets the threshold.

```sh
# print spans and metrics to stderr for a single run
cerebrai --print-telemetry version

# export to a collector
export OTEL_EXPORTER_OTLP_ENDPOINT=https://collector.example.com:4317
export OTEL_EXPORTER_OTLP_HEADERS="api-key=..."
cerebrai version
```

The desktop app instead has an OTLP export checkbox in the Developer section
of its Preferences window, which `CEREBRAI_DEV_MODE` reveals; without it,
spans and metrics print to stderr. The log level comes from
`CEREBRAI_LOG_LEVEL`, which applies whether or not that section is visible.
See [Configuration](#configuration).

### Chat content logging

The desktop app logs chat exchanges as metadata only (message lengths).
Conversation content is the user's private memory store and inbox, and in
OTLP mode log records leave the machine for whatever collector
`OTEL_EXPORTER_OTLP_ENDPOINT` names, so raw text is deliberately kept out of
telemetry in any build a user might run.

For local development, build with the `cerebrai_dev` tag to log the full
message and reply text instead:

```sh
make run-desktop                        # dev build, tag applied for you
make install-macos                      # dev build, tag applied for you
go run -tags cerebrai_dev ./cmd/cerebrai-desktop
```

Two gates have to be open for the text to actually appear: the binary must
be built with `cerebrai_dev`, **and** `CEREBRAI_LOG_LEVEL=debug` must be
set. A dev build says so in the Developer section of its Preferences window.
`make build-desktop` and `make package-macos` never apply the tag — do not
build release artifacts with it.

## Docker

```sh
docker build -t cerebrai .
docker run --rm cerebrai version
```

## Credits

App icon: <a href="https://www.flaticon.com/free-icons/brain" title="brain icons">Brain icons created by Reddie - Flaticon</a>

