# cerebrai

<img src="build/macos/icon.png" alt="CerebrAI app icon" width="120" align="right">

cerebrai is a personal automation system and "second brain," described in
full in [DESIGN.md](DESIGN.md). The product surface is a native macOS
desktop app; automation execution, scheduling, and the memory store run
in-process behind its UI, reached through a single `app.Client` port. The
CLI in this repo is a debugging tool, not the primary interface.

## Layout

- `cmd/cerebrai/` — CLI entrypoint (debugging tool, see DESIGN.md §3)
- `cmd/cerebrai-desktop/` — desktop app entrypoint; in a developer build it
  also starts the in-process MCP server (below) and points the chat provider
  at it
- `internal/cli/` — cobra command tree (`version`, `db-migrate`)
- `internal/app/` — the domain types (`Session`, `Message`, `Automation`)
  and the `Client` port the desktop UI is written against. The UI holds no
  automation, memory, or LLM logic of its own; it only calls this interface
  (DESIGN.md §3). The concrete implementation lives elsewhere so swapping it
  never touches the UI.
- `internal/desktopui/` — native desktop UI (Chat and Automations tabs),
  built with [Fyne](https://fyne.io)
- `internal/storage/` — cerebrai's on-disk persistence: where the SQLite
  database lives (see [Data storage](#data-storage)), bringing its schema up
  to date, and the `SQLite` `app.Client` implementation that reads and
  writes its tables — the implementation the desktop app runs against today
- `internal/chat/` — the plain, non-agentic conversation seam
  (`ConversationProvider`: one reply per user message, no tool loop) plus
  the per-session chat model catalog (DESIGN.md §5.2)
- `internal/automationagent/` — the automation writer agent loop, built on
  [Eino](https://github.com/cloudwego/eino)'s ReAct agent: the genuinely
  agentic loop that authors or edits an automation, invoked by the chat
  handoff rather than wrapped around every chat turn (DESIGN.md §5.3)
- `internal/telemetry/` — OpenTelemetry (traces, metrics, logs) setup, plus
  `LogChatExchange` whose payload is `cerebrai_dev`-gated (see
  [Chat content logging](#chat-content-logging))
- `internal/config/` — build metadata injected via `-ldflags`, and the
  `CEREBRAI_LOG_LEVEL` / `CEREBRAI_DB_PATH` env var names
- `internal/devmode/` — everything active only in a developer's checkout:
  the `CEREBRAI_DEV_MODE` gate (`devmode.Enabled`), the dev-only model
  catalog and local Claude Code CLI provider (`devmode/claudecode`), and the
  in-process MCP server (`devmode/devmcp`) that serves cerebrai's own
  `create_automation` / `edit_automation` tools to that CLI (DESIGN.md §5.6)

The chat seam and the automation writer agent are scaffolded and, in a
developer build, run end-to-end against the local `claude` CLI.
Schedule/trigger evaluation, automation execution, and the memory store
aren't built yet; they'll run in-process behind the same `app.Client` port.
Today `cmd/cerebrai-desktop` opens the SQLite database directly and wires
`storage.SQLite` in as that `app.Client`.

## Data storage

The desktop app persists its data — automations (including the writer
agent's authored source), and chat sessions with their message history and
resume handle — in a SQLite database, opened via `internal/storage.Open`.
Where that database lives
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

`cmd/cerebrai-desktop` opens this database directly and wires
`storage.SQLite` in as its `app.Client`, so automations and chat history
survive restarts.

The schema is applied from embedded migrations on every launch, and in a
developer build the seed data is applied too. cerebrai is pre-release, so
the schema is not versioned incrementally — there is one migration file,
edited in place (see [CLAUDE.md](CLAUDE.md)). After editing it, run `make
clean-db` to drop the local database, then `cerebrai db-migrate` (or just
launch the app) to rebuild it from scratch.

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
make tidy          # go mod tidy
make clean         # remove bin/ and dist/
make clean-db      # delete the dev-mode SQLite database (see Data storage)
make docker-ci     # reproduce the CI build/vet/test job in a container with the Fyne cgo deps

make package-macos # wrap bin/cerebrai-desktop in dist/macos/CerebrAI.app (add DMG=1 for an installer)
make install-macos # build + overwrite the CerebrAI.app in ~/Applications, as a dev build (see below)
```

macOS packaging details are in [build/macos/README.md](build/macos/README.md).

### CLI

The CLI (`cmd/cerebrai`) is a debugging tool, not the product surface
(DESIGN.md §3). Its commands:

```sh
cerebrai version     # print the build version, commit, and date
cerebrai db-migrate  # bring the SQLite schema up to date (and seed data, in a dev build)
```

Both take the persistent `--log-level` and `--print-telemetry` flags (see
[Telemetry](#telemetry)).

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

The CLI takes its log level and telemetry mode from flags (`--log-level`,
`--print-telemetry`) rather than `CEREBRAI_LOG_LEVEL` or a Preferences
toggle, but `cerebrai db-migrate` still resolves the database the same way
the app does, so `CEREBRAI_DEV_MODE` and `CEREBRAI_DB_PATH` apply to it. See
[CLI](#cli) and [Telemetry](#telemetry).

## Telemetry

The CLI exports spans, metrics, and logs via OTLP/gRPC whenever an OTLP
collector endpoint is configured — `OTEL_EXPORTER_OTLP_ENDPOINT`, which a
developer's checkout sets in `.env`. If that collector is unreachable the
export fails silently after a short bounded timeout and never fails the
command itself.

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

The automation writer agent and the dev-mode MCP server are instrumented
too: each automation-agent run opens its own root trace, and every incoming
MCP method call is wrapped in a span, all exported through the same global
provider.

[observability/](observability/README.md) has a local dev stack (OTel
Collector, Zipkin, Prometheus, Grafana, Loki) deployed as a Helm chart for
receiving that OTLP data.

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

`Dockerfile` builds a minimal static image of the CLI (`cmd/cerebrai`
only — the Fyne desktop app needs cgo and system graphics libraries):

```sh
docker build -t cerebrai .
docker run --rm cerebrai version
```

`Dockerfile.ci` (via `make docker-ci`) is separate: it reproduces the CI
build/vet/test job — including the desktop build's cgo dependencies — in a
container, so that job can be verified locally without installing those
libraries on the host.

## Credits

App icon: <a href="https://www.flaticon.com/free-icons/brain" title="brain icons">Brain icons created by Reddie - Flaticon</a>

