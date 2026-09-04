VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

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

.PHONY: run-desktop
run-desktop:
	go run ./cmd/cerebrai-desktop

.PHONY: test
test:
	go test ./...

.PHONY: vet
vet:
	go vet ./...

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
