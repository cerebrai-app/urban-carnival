# syntax=docker/dockerfile:1

FROM golang:1.27-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags "-s -w \
      -X github.com/cerebrai-app/urban-carnival/internal/config.Version=${VERSION} \
      -X github.com/cerebrai-app/urban-carnival/internal/config.Commit=${COMMIT} \
      -X github.com/cerebrai-app/urban-carnival/internal/config.Date=${DATE}" \
    -o /out/cerebrai ./cmd/cerebrai

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/cerebrai /usr/local/bin/cerebrai
ENTRYPOINT ["/usr/local/bin/cerebrai"]
