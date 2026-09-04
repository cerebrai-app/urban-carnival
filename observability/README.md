# Observability stack

Local dev stack for traces (Zipkin), metrics (Prometheus + Grafana), and
logs (Loki), fed by an OTel Collector that receives OTLP from `cerebrai`.
Deployed as a Helm chart to the local Kubernetes cluster (Rancher Desktop).
Everything — including the collector — is reachable through Ingress; nothing
needs `kubectl port-forward`.

Zipkin stores spans in Cassandra (`openzipkin/zipkin-cassandra`, a single-node
Cassandra with the zipkin2 schema pre-loaded) rather than its default
in-memory store, so traces survive a Zipkin pod restart. It runs as a
`StatefulSet` with a `PersistentVolumeClaim` for `/var/lib/cassandra`. A
`zipkin-dependencies` `CronJob` (schedule in `values.yaml`, hourly by default)
runs the batch job that computes the service dependency graph shown on
Zipkin's "Dependencies" tab — Zipkin itself only ever writes/reads raw spans,
it doesn't compute that graph on the fly.

Logs are aggregated in Loki (`grafana/loki`), running single-binary with
filesystem storage — like Prometheus, it has no `PersistentVolumeClaim`, so
log history doesn't survive a pod restart. The OTel Collector forwards logs
it receives over OTLP to Loki's native OTLP endpoint (`/otlp/v1/logs`) via
an `otlphttp` exporter — no separate `loki` exporter or Promtail needed.
When run with `--otlp`, `cerebrai`'s log lines already carry `trace_id`/
`span_id` — the `otelslog` bridge in `internal/telemetry/telemetry.go`
attaches the active span's context to every record automatically — so
Grafana's trace-to-logs linking works out of the box.

## Install

```sh
helm install observability ./chart --namespace cerebrai-monitoring --create-namespace
```

This also drops a `HelmChartConfig` into `kube-system` (see
`chart/templates/traefik-entrypoints.yaml`) that adds dedicated Traefik
entryPoints for the OTel Collector's OTLP ports — see "Why dedicated ports
for the collector, and why not `*.localtest.me`" below. It triggers a
one-time reinstall of k3s's built-in Traefik add-on; give it a few seconds
after `helm install`/`upgrade` before the new ports are live.

Add these to `/etc/hosts` (see below for why they're bare names, not FQDNs):

```
127.0.0.1 zipkin prometheus grafana loki otel-collector
```

Then run cerebrai with `--otlp`, pointing it at the collector through
ingress:

```sh
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4317 \
  go run ./cmd/cerebrai --otlp ...
```

## Upgrade / iterate

The Grafana dashboard JSON lives in `chart/templates/grafana-config.yaml` as a
ConfigMap (Kubernetes has no bind mounts). Edit it, then:

```sh
helm upgrade observability ./chart --namespace cerebrai-monitoring
```

A checksum annotation on the Grafana pod spec forces a rollout whenever that
file changes, so the new dashboard shows up within a few seconds — no manual
restart needed.

## UIs

- Zipkin: http://zipkin (routes via Traefik's default port 80)
- Prometheus: http://prometheus (port 80)
- Grafana: http://grafana (port 80, anonymous admin access, local-only
  stack) — Prometheus, Zipkin, and Loki datasources are pre-provisioned,
  plus a `cerebrai overview` dummy dashboard to iterate on.
- Loki: http://loki (port 80) — query via Grafana's Explore view, or
  `logcli --addr=http://loki query '{...}'`.
- OTel Collector: http://otel-collector:4318 (OTLP/HTTP), http://otel-collector:4317
  (OTLP/gRPC, h2c) — its own dedicated ports, see below.

## Why dedicated ports for the collector, and why not `*.localtest.me`

Two separate local-machine quirks shaped this setup — worth recording so a
future edit doesn't accidentally reintroduce either one:

1. **The collector uses 4317/4318, nothing else does.** Those are the
   standard OTLP ports `cerebrai` already defaults to when unconfigured (see
   `internal/telemetry/telemetry.go`), so serving the collector there isn't
   a workaround — it's matching the SDK's zero-config expectation. Zipkin,
   Prometheus, and Grafana don't have an equivalent default to match, so
   they route through Traefik's default `web` (80) entrypoint like any
   other Ingress — an earlier version of this chart gave them dedicated
   ports too, on a mistaken theory (below) that plain 80/443 was
   unreliable; testing showed 80 works fine once the *hostname* problem
   (also below) was fixed, so that indirection was removed.

2. **Not `*.localtest.me`.** That's the likely culprit if a URL here ever
   comes back `ERR_CONNECTION_RESET` (or hangs indefinitely) in a browser
   but works fine with `curl`: some machines run a local HTTP/HTTPS proxy
   (check `scutil --proxy` on macOS) that resets connections to hostnames
   resolving to loopback, as an anti-SSRF safeguard — which is exactly what
   `*.localtest.me` does. `curl` doesn't hit this (it ignores the system
   proxy by default), which is why it can look "fixed" from the terminal
   while a browser still fails. The workaround here: macOS's proxy config
   commonly sets `ExcludeSimpleHostnames`, which skips the proxy for
   single-label hostnames (no dot) — hence bare `/etc/hosts` names like
   `zipkin` instead of `zipkin.localtest.me`.

## Uninstall

```sh
helm uninstall observability --namespace cerebrai-monitoring
```

This also removes the `traefik` `HelmChartConfig` in `kube-system` — Helm
tracks every resource a release creates regardless of namespace — which
triggers another Traefik reinstall back to its default ports/entrypoints.
