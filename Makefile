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
# release artifact with it; see internal/devmode/chatlog_dev.go.
DEV_TAG := cerebrai_dev

# Build tags for the desktop binary. Empty (release) by default; install-macos
# overrides it to DEV_TAG so the app it installs matches run-desktop.
DESKTOP_TAGS ?=

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
	go build -trimpath -ldflags "$(LDFLAGS)" $(if $(DESKTOP_TAGS),-tags $(DESKTOP_TAGS)) -o bin/cerebrai-desktop ./cmd/cerebrai-desktop

# Runs against the SQLite-backed workerclient (see internal/storage), with
# full chat content logging compiled in (visible at the debug log level, set
# in Preferences). CEREBRAI_DEV_MODE also keeps the database in the repo
# instead of the OS's per-user application data directory (internal/storage.Path).
.PHONY: run-desktop
run-desktop:
	CEREBRAI_DEV_MODE=1 go run -tags $(DEV_TAG) ./cmd/cerebrai-desktop

# macOS app bundle. Wraps bin/cerebrai-desktop in CerebrAI.app under
# dist/macos; on its own it uses the release binary (no dev tag, no baked-in
# environment). Pass DMG=1 to also build the .dmg installer (any non-empty
# value other than 0). macOS only; see build/macos/README.md.
APP_NAME    ?= CerebrAI
INSTALL_DIR ?= $(HOME)/Applications

# LSEnvironment entries baked into the packaged app's Info.plist (applied on
# a Finder/Dock launch). MACOS_APP_ENV_NAMES lists environment variable names
# whose values the recipe reads straight from its own environment (the
# Makefile exports every name .env defines), so a value with spaces is fine
# — only the names get word-split here, and env var names can't contain
# whitespace. Empty for plain release packaging; install-macos fills it in.
# MACOS_DB_PATH is passed separately because the pin below is computed here,
# not exported from .env; it's expanded inside shell quotes, so a checkout
# path with spaces survives too.
MACOS_APP_ENV_NAMES ?=
MACOS_DB_PATH       ?=

# bash for the array + ${!name} indirection; this path is macOS/CI only and
# package-app.sh is already bash.
package-macos install-macos: SHELL := /bin/bash

.PHONY: package-macos
package-macos: build-desktop
	args=(); \
	$(if $(MACOS_APP_ENV_NAMES),for n in $(MACOS_APP_ENV_NAMES); do args+=(--env "$$n=$${!n}"); done;) \
	$(if $(MACOS_DB_PATH),args+=(--env "CEREBRAI_DB_PATH=$(MACOS_DB_PATH)");) \
	build/macos/package-app.sh --exe bin/cerebrai-desktop --version "$(VERSION)" \
		--name "$(APP_NAME)" --outdir dist/macos $(if $(filter-out 0,$(DMG)),--dmg,) \
		"$${args[@]}"

# The dev-mode variables baked into the installed app: every CEREBRAI_* /
# OTEL_* name .env defines, plus CEREBRAI_DB_PATH pinned to this checkout's
# database (a Finder launch runs with working directory /, where the plain
# CEREBRAI_DEV_MODE ./cerebrai.db would be unwritable and abort startup). Any
# CEREBRAI_DB_PATH in .env is ignored in favor of this pin.
INSTALL_MACOS_ENV_NAMES := $(filter-out CEREBRAI_DB_PATH,$(filter CEREBRAI_% OTEL_%,$(ENV_NAMES)))

# Overwrite the locally installed app with a fresh local build, configured as
# a dev build: DEV_TAG for full chat content logging (matching run-desktop),
# plus the dev-mode env above for the Developer preferences section, debug
# logging, and the checkout's database. Installs to the per-user
# ~/Applications (no admin prompt); override with INSTALL_DIR. Quits a
# running copy first so the replacement takes effect on next launch. The
# fresh bundle is copied in beside the old one and only then swapped in
# (old renamed aside, new renamed into place, old deleted), so a failure
# before the swap leaves the existing install untouched.
.PHONY: install-macos
install-macos: MACOS_APP_ENV_NAMES = $(INSTALL_MACOS_ENV_NAMES)
install-macos: MACOS_DB_PATH = $(CURDIR)/cerebrai.db
install-macos: DESKTOP_TAGS = $(DEV_TAG)
install-macos: package-macos
	- osascript -e 'quit app "$(APP_NAME)"' 2>/dev/null
	mkdir -p "$(INSTALL_DIR)"
	rm -rf "$(INSTALL_DIR)/$(APP_NAME).app.new" "$(INSTALL_DIR)/$(APP_NAME).app.old"
	cp -R "dist/macos/$(APP_NAME).app" "$(INSTALL_DIR)/$(APP_NAME).app.new"
	if [ -d "$(INSTALL_DIR)/$(APP_NAME).app" ]; then mv "$(INSTALL_DIR)/$(APP_NAME).app" "$(INSTALL_DIR)/$(APP_NAME).app.old"; fi
	mv "$(INSTALL_DIR)/$(APP_NAME).app.new" "$(INSTALL_DIR)/$(APP_NAME).app"
	rm -rf "$(INSTALL_DIR)/$(APP_NAME).app.old"
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
