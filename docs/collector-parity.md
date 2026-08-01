# OpenTelemetry Collector config parity — gaps & plan

## Goal & scope

mkot mirrors the OpenTelemetry **Collector's configuration schema** so that a
collector `otlp` exporter block — and the processor/pipeline config around it —
can be lifted into an mkot config with as few edits as possible.

mkot is **not** a collector. It configures the OpenTelemetry Go **SDK**
in-process, so it has no receivers, connectors, or the collector runtime. What
it mirrors is the *shape and semantics* of the exporter, processor, and pipeline
config, wherever the SDK can honor it.

**Invariant (already upheld):** any collector setting the SDK cannot honor is
rejected with an error — on the signals it applies to — rather than silently
dropped. A lifted config fails loudly at startup instead of behaving differently
than it did in the collector. See [README.md](../README.md) "Not supported".

This document lists what is **under-implemented** relative to the collector, so
that closing the list is a matter of working through it.

### Status legend

| | meaning |
|---|---|
| ✅ | implemented, semantics match the collector |
| ⚠️ | implemented but **diverges** — safe reuse needs the note |
| ❌ | not implemented, but the SDK *could* express it — a real TODO |
| 🚫 | cannot be implemented with the OTel Go SDK — rejected if set, documented |

---

## 1. Config file structure (top level) — the biggest reuse gap

A collector file and an mkot file differ at the top level, so a config is **not**
liftable whole today; only the `exporters:`/`processors:` *blocks* transfer.

| collector | mkot | status | note |
|---|---|---|---|
| `receivers:` | — | 🚫 | the app itself is the telemetry source |
| `exporters:` | `exporters:` | ✅ | same key |
| `processors:` | `processors:` | ✅ | same key, limited set — see §9 |
| `extensions:` | — | ❌ | needed for auth / storage — see §4, §5 |
| `connectors:` | — | 🚫 | no in-SDK equivalent |
| `service.pipelines.traces` | `providers.tracer` | ⚠️ | **different key path and signal names** (`traces/metrics/logs` → `tracer/meter/logger`) |
| `service.pipelines.<x>.receivers` | — | 🚫 | n/a |
| `service.telemetry` | — | ❌ | self-observability config |
| `${env:VAR}` / `${file:path}` substitution | — | ❌ | **secrets/endpoints from env do not resolve** |

Confirmed in `config.go` (top-level keys `enabled`/`processors`/`exporters`/`providers`)
and `resolver.go:122,199,289` (`tracer`/`meter`/`logger` ids). No env expansion
exists on the unmarshal path.

**Plan.** Either (a) accept `service.pipelines` + `traces/metrics/logs` as
aliases that map onto `providers`/`tracer…`, or (b) document the mapping as the
one required edit. Add `${env:...}` substitution before unmarshal (P1) — without
it, the ubiquitous `endpoint: ${env:OTEL_EXPORTER_OTLP_ENDPOINT}` /
`headers: {authorization: ${env:TOKEN}}` collector idiom silently becomes a
literal string.

---

## 2. Transport & endpoint (`exporters.otlp`)

| collector field | status | note |
|---|---|---|
| `endpoint` | ✅ | host:port, `http(s)://` URL, and gRPC targets (`dns:///`, `unix:///`, `xds:///`); under `protocol: http` it is a base URL and `/v1/<signal>` is appended |
| `protocol` (`grpc` / `http/protobuf`) | ✅ | |
| `encoding` (`proto` / `json`, http) | 🚫 | the OTel Go SDK http exporters are protobuf-only; there is no `json` option to expose |
| `proxy_url` (http) | ✅ | routes HTTP exports through a proxy; rejected under grpc |
| `service_config` (grpc) | ✅ | raw gRPC service-config JSON; mutually exclusive with `balancer_name` |
| `compression` | ⚠️ / 🚫 | only `gzip` (the SDK/grpc register nothing else); `zstd`/`snappy`/`zlib`/`deflate`/`lz4` are **rejected**. `none` defers to `OTEL_EXPORTER_OTLP_COMPRESSION` when that env var is set |

---

## 3. TLS (`configtls`) — near-complete

| collector field | status |
|---|---|
| `ca_file` / `ca_pem`, `cert_file` / `cert_pem`, `key_file` / `key_pem` | ✅ |
| `insecure`, `insecure_skip_verify`, `server_name_override` | ✅ |
| `include_system_ca_certs_pool` | ✅ |
| `min_version`, `max_version` | ✅ (pair validated) |
| `cipher_suites` | ✅ (TLS 1.2 only — Go ignores it at 1.3; insecure suites rejected) |
| `curve_preferences` | ✅ |
| `reload_interval` | ⚠️ reloads the **client cert** only; the collector also reloads the CA pool |
| `tpm` | 🚫 not in the struct |

**Plan.** Extend `reload_interval` to re-read the CA pool too (P3), or document
that only the client keypair rotates.

---

## 4. Auth (`configauth` + extensions) — highest-value gap

Only static `headers` are supported (`otlp/exporter.go:83` leaves `auth`
commented out). Authenticated backends that need a refreshing credential cannot
be reused:

| collector authenticator | status | SDK path |
|---|---|---|
| `bearertokenauth` (static token) | ✅ via `headers` | — |
| `bearertokenauth` (token from file, refreshed) | ❌ | `WithDialOption(grpc.WithPerRPCCredentials(...))` |
| `oauth2clientauth` | ❌ | PerRPCCredentials with an `golang.org/x/oauth2` token source |
| `basicauth` | ❌ | a static `Authorization: Basic` header, or PerRPCCredentials |

**Plan (P1).** Add an `auth:` block on the exporter that maps to a
`PerRPCCredentials` (gRPC) / `RoundTripper` (HTTP). Start with `bearertokenauth`
(file + refresh) and `basicauth`; `oauth2clientauth` next.

---

## 5. Sending queue & batching (`exporterhelper.sending_queue`)

| collector field | status | note |
|---|---|---|
| `enabled` | ✅ | |
| `queue_size` | ⚠️ | counted in **spans/records**, not the collector's requests/items/bytes |
| `sizer` (`requests`/`items`/`bytes`) | 🚫 | SDK counts items only |
| `num_consumers` | ⚠️ | only `1` (the SDK batcher is single-consumer); `>1` rejected |
| `block_on_overflow` | ⚠️ | traces ✅ (`trace.WithBlocking`); logs 🚫 rejected (log batcher always drops) |
| `wait_for_result` | 🚫 | the SDK batcher is async |
| `batch.flush_timeout`, `batch.max_size` | ✅ | |
| `batch.min_size` | 🚫 | no minimum-batch concept in the SDK |
| `batch.sizer`, `batch.partition` | 🚫 | |
| `storage` (persistent/durable queue) | 🚫 | no durable SDK queue — buffered data is lost on crash |
| applies to **metrics** | ⚠️ | ignored on a metric-only exporter (metrics use the periodic reader; set `interval`) |

Semantic caveat: the collector `sending_queue` is an async retry queue in front
of the exporter; mkot maps it onto the SDK **batch processor**, a bounded
in-memory buffer. Capacity and overflow behavior are close but not identical.

---

## 6. Retry (`retry_on_failure`)

| collector field | status |
|---|---|
| `enabled`, `initial_interval`, `max_interval`, `max_elapsed_time` | ✅ |
| `randomization_factor`, `multiplier` | 🚫 the SDK's backoff factors are fixed — rejected |

⚠️ Divergence: with a partial block, `max_elapsed_time` unset means **0 =
retry forever**, unlike the collector's 5m default. Documented in `retry.go`.

---

## 7. Timeout (`exporterhelper.timeout`)

`timeout` ✅ is wired for all three signals and the metric reader.

⚠️ **Ceiling:** on **traces and logs** a value above **30s** is capped by the
batch processor's own 30s export deadline (metrics are aligned and uncapped; a
`sending_queue: {enabled: false}` simple processor is uncapped). Documented on
the `Timeout` field.

**Plan (P2).** Thread an export-timeout option into
`QueueConfig.BuildSpanProcessor` / `BuildLogProcessor`
(`trace.WithExportTimeout` / `log.WithExportTimeout`). Deferred because `otlp/`
is a nested module pinning a published `mkot`, and the per-module `GOWORK=off`
CI would fail until `mkot` is republished and the pin bumped.

---

## 8. Metrics (SDK-side, beyond the collector exporter schema)

| knob | status | note |
|---|---|---|
| `interval` (push period) | ✅ | SDK owns the cadence; not a collector exporter field |
| `temporality` (`cumulative`/`delta`/`lowmemory`) | ✅ | |
| `exemplar_filter` (`trace_based`/`always_on`/`always_off`) | ✅ | |
| views (histogram buckets, instrument rename/drop, attribute/cardinality limits) | ❌ | SDK `metric.View`; collector does this in `filter`/`transform`/`metricstransform` processors |
| aggregation selector | ❌ | `otlpmetricgrpc.WithAggregationSelector` |
| external `Producer`s (bridges) | ❌ | `metric.WithProducer` |

**Plan (P3).** Expose a `views:` config and an aggregation selector.

---

## 9. Processors

Registered today: `resource`, `sampler` (`resource.go:105`, `sampler.go:86`).
A collector `processors:` entry of any other type **fails at unmarshal**
(`"unknown type"`), which is the loud-failure behavior we want but also the reuse
gap.

| collector processor | mkot | status |
|---|---|---|
| `resource` / `resourcedetection` | `resource` (attributes + detectors) | ✅ |
| `probabilistic_sampler` (head) | `sampler` (`trace_id_ratio`/`parent_based`) | ✅ |
| `tail_sampling` | — | 🚫 tail sampling is not feasible in-process |
| `batch` | folded into `exporters.otlp.sending_queue.batch` | ⚠️ no standalone `batch` processor — a `processors: [batch]` config does not map |
| `memory_limiter` | — | 🚫 collector-runtime concern |
| `filter` / `transform` / `attributes` / `metricstransform` | — | ❌ (spans/logs via SDK processors; metrics via views) |
| `k8sattributes`, `span`, `groupbyattrs`, … | — | ❌ |

**Plan.** A `batch` processor alias that forwards to the exporter's batch config
(P2). `filter`/`transform` are larger (P3).

---

## 10. gRPC / HTTP client knobs (`configgrpc` / `confighttp`)

| field | status |
|---|---|
| `keepalive` (`time`/`timeout`/`permit_without_stream`) | ✅ (sub-10s `time` rejected — grpc clamps it) |
| `read_buffer_size`, `write_buffer_size`, `wait_for_ready`, `authority`, `balancer_name` | ✅ |
| `reconnection_period` | ✅ (SDK, not a collector field) |
| full gRPC service config (retry/health/method policy) | ✅ `service_config` (raw JSON); mutually exclusive with `balancer_name` |
| `middlewares` | ❌ (`otlp/exporter.go`, commented out) — no interceptor hook |
| `proxy_url` (http) | ✅ (HTTP only; rejected under grpc, which uses the env proxy) |
| reuse a pre-built `*grpc.ClientConn` / attach interceptors | 🚫 not YAML-expressible |

Under `protocol: http` the gRPC-only knobs are rejected, not ignored.

---

## 11. Semantic divergences — read before lifting a config

Settings that look identical but behave differently. These are the dangerous
ones for "just reuse the file":

1. **Pipeline wiring** — `service.pipelines.{traces,metrics,logs}` →
   `providers.{tracer,meter,logger}` (§1).
2. **No `${env:...}` substitution** — env-templated endpoints/tokens become
   literal strings (§1).
3. **`sending_queue`** is an SDK batch buffer, not an async retry queue; and
   `queue_size` is in spans/records regardless of any `sizer` (§5).
4. **`retry_on_failure.max_elapsed_time: 0`** means forever, vs the collector's
   bounded 5m default (§6).
5. **`timeout` > 30s** is capped on traces/logs (§7).
6. **`compression: none`** defers to `OTEL_EXPORTER_OTLP_COMPRESSION` (§2).
7. **`sending_queue` on metrics** is ignored — use `interval` (§5).

---

## Roadmap

### P1 — unlocks whole-file reuse
- [ ] `${env:VAR}` (and ideally `${file:path}`) substitution before unmarshal
- [ ] Accept `service.pipelines` + `traces/metrics/logs` as aliases for `providers`/`tracer…` (or document the one-line mapping)
- [ ] `auth:` block → bearer (file+refresh) / basic / oauth2 via PerRPCCredentials + HTTP RoundTripper

### P2 — common knobs
- [ ] Lift the 30s trace/log timeout ceiling (export-timeout through `QueueConfig`) — after an `mkot` republish
- [ ] `batch` standalone processor alias → exporter batch config
- [x] `proxy_url` on the HTTP transport
- [x] `service_config` on the gRPC transport (moved up from P3)

### P3 — advanced
- [ ] Metric `views:` (bucket boundaries, rename/drop, cardinality limits) + aggregation selector
- [ ] `filter` / `transform` / `attributes` processors — *metrics only*; span/log attribute mutation is not SDK-expressible (reclassified toward "won't do")
- [ ] TLS CA-pool reload on `reload_interval`

### Won't do — no OTel Go SDK equivalent (rejected + documented)
- Persistent `storage` sending queue · `tail_sampling` · `memory_limiter` ·
  TLS `tpm` · queue `sizer` / `wait_for_result` / `num_consumers > 1` /
  `batch.min_size` · retry `randomization_factor` / `multiplier` ·
  non-gzip compression · OTLP/HTTP `encoding: json` (SDK is protobuf-only) ·
  span/log `filter`/`transform` attribute mutation · `receivers` / `connectors`.
