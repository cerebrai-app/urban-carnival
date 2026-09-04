VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Local developer environment. .env is gitignored and optional; only the
# names it defines are exported, so it cannot clobber other make variables.
# See README.md for the variables the app reads.
ifneq (,$(wildcard .env))
include .env
export $(shell sed -n 's/^[[:space:]]*\([A-Za-z_][A-Za-z0-9_]*\)[[:space:]]*=.*/\1/p' .env)
endif

# DEV_TAG builds the local-development variant of the desktop app, which
# logs full chat message and reply text at the debug level. Never build a
# release artifact with it; see internal/desktopui/chatlog_dev.go.
DEV_TAG := cerebrai_dev

LDFLAGS := -s -w \
	-X github.com/cerebrai-app/urban-carnival/internal/version.Version=$(VERSION) \
	-X github.com/cerebrai-app/urban-carnival/internal/version.Commit=$(COMMIT) \
	-X github.com/cerebrai-app/urban-carnival/internal/version.Date=$(DATE)

.PHONY: build
build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/cerebrai ./cmd/cerebrai

.PHONY: run
run:
	go run ./cmd/cerebrai

.PHONY: build-desktop
build-desktop:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/cerebrai-desktop ./cmd/cerebrai-desktop

# Runs against the mock worker client, with full chat content logging
# compiled in (visible at the debug log level, set in Preferences).
.PHONY: run-desktop
run-desktop:
	go run -tags $(DEV_TAG) ./cmd/cerebrai-desktop

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
	rm -rf bin

# Reproduces the CI build-test job (build, vet, test) in a container with
# the Fyne/glfw cgo dependencies installed, matching the ubuntu-latest
# runner without requiring those libraries on the host.
.PHONY: docker-ci
docker-ci:
	docker build -f Dockerfile.ci .
