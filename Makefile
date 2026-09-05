VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Local developer environment. .env is gitignored and optional; only the
# names it defines are exported, so it cannot clobber other make variables.
# See README.md for the variables the app reads.
ENV_FILE   := .env
ENV_NAMES  := $(if $(wildcard $(ENV_FILE)),$(shell sed -n 's/^[[:space:]]*\([A-Za-z_][A-Za-z0-9_]*\)[[:space:]]*=.*/\1/p' $(ENV_FILE)))
ifneq (,$(wildcard $(ENV_FILE)))
include $(ENV_FILE)
export $(ENV_NAMES)
endif

# DEV_TAG builds the local-development variant of the desktop app, which
# logs full chat message and reply text at the debug level. Never build a
# release artifact with it; see internal/desktopui/chatlog_dev.go.
DEV_TAG := cerebrai_dev

LDFLAGS := -s -w \
	-X github.com/cerebrai-app/urban-carnival/internal/config.Version=$(VERSION) \
	-X github.com/cerebrai-app/urban-carnival/internal/config.Commit=$(COMMIT) \
	-X github.com/cerebrai-app/urban-carnival/internal/config.Date=$(DATE)

.PHONY: build
build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/cerebrai ./cmd/cerebrai

.PHONY: run
run:
	go run ./cmd/cerebrai

.PHONY: build-desktop
build-desktop:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/cerebrai-desktop ./cmd/cerebrai-desktop

# Runs against the SQLite-backed workerclient (see internal/storage), with
# full chat content logging compiled in (visible at the debug log level, set
# in Preferences). CEREBRAI_DEV_MODE also keeps the database in the repo
# instead of the OS's per-user application data directory (internal/storage.Path).
.PHONY: run-desktop
run-desktop:
	CEREBRAI_DEV_MODE=1 go run -tags $(DEV_TAG) ./cmd/cerebrai-desktop

# macOS app bundle. Wraps the release-style desktop binary (no dev tag) in
# CerebrAI.app under dist/macos. Pass DMG=1 to also build the .dmg installer
# (any non-empty value other than 0). macOS only; see build/macos/README.md.
APP_NAME    ?= CerebrAI
INSTALL_DIR ?= $(HOME)/Applications

# Environment variables baked into the packaged app's Info.plist as
# LSEnvironment (applied on a Finder/Dock launch). Empty for the plain
# release packaging path; install-macos fills in the dev-mode vars below.
MACOS_APP_ENV ?=

.PHONY: package-macos
package-macos: build-desktop
	build/macos/package-app.sh --exe bin/cerebrai-desktop --version "$(VERSION)" \
		--name "$(APP_NAME)" --outdir dist/macos $(if $(filter-out 0,$(DMG)),--dmg,) \
		$(foreach kv,$(MACOS_APP_ENV),--env $(kv))

# The dev-mode variables baked into the installed app: every CEREBRAI_* /
# OTEL_* name .env defines, plus CEREBRAI_DB_PATH pinned to this checkout's
# database (a Finder launch runs with working directory /, where the plain
# CEREBRAI_DEV_MODE ./cerebrai.db would be unwritable and abort startup). Any
# CEREBRAI_DB_PATH in .env is ignored in favor of this pin.
INSTALL_MACOS_ENV_NAMES := $(filter-out CEREBRAI_DB_PATH,$(filter CEREBRAI_% OTEL_%,$(ENV_NAMES)))
INSTALL_MACOS_ENV := $(foreach n,$(INSTALL_MACOS_ENV_NAMES),$(n)=$($(n))) CEREBRAI_DB_PATH=$(CURDIR)/cerebrai.db

# Overwrite the locally installed app with a fresh local build, configured as
# a dev build via INSTALL_MACOS_ENV above (Developer preferences section,
# debug logging, database in the checkout). Installs to the per-user
# ~/Applications (no admin prompt); override with INSTALL_DIR. Quits a
# running copy first so the replacement takes effect on next launch. The new
# bundle is copied in beside the old one and swapped in only once the copy
# succeeds, so a failure can't leave you with no installed app.
.PHONY: install-macos
install-macos: MACOS_APP_ENV = $(INSTALL_MACOS_ENV)
install-macos: package-macos
	- osascript -e 'quit app "$(APP_NAME)"' 2>/dev/null
	mkdir -p "$(INSTALL_DIR)"
	rm -rf "$(INSTALL_DIR)/$(APP_NAME).app.new"
	cp -R "dist/macos/$(APP_NAME).app" "$(INSTALL_DIR)/$(APP_NAME).app.new"
	rm -rf "$(INSTALL_DIR)/$(APP_NAME).app"
	mv "$(INSTALL_DIR)/$(APP_NAME).app.new" "$(INSTALL_DIR)/$(APP_NAME).app"
	@echo "installed $(INSTALL_DIR)/$(APP_NAME).app"

.PHONY: test
test:
	go test ./...

.PHONY: vet
vet:
	go vet ./...
	go vet -tags $(DEV_TAG) ./...

.PHONY: lint
lint:
	golangci-lint run

.PHONY: fmt
fmt:
	gofmt -l -w .

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: clean
clean:
	rm -rf bin dist

# Reproduces the CI build-test job (build, vet, test) in a container with
# the Fyne/glfw cgo dependencies installed, matching the ubuntu-latest
# runner without requiring those libraries on the host.
.PHONY: docker-ci
docker-ci:
	docker build -f Dockerfile.ci .
