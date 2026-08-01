# mkot

*mkot* is a utility designed to simplify the creation of OpenTelemetry Providers.
It supports configuration patterns that closely align with those used in opentelemetry-collector, enabling seamless integration and familiar setup for observability pipelines.

## Peek

From:
```yaml
enabled: true
processors:
  sampler:
    type: trace_id_ratio
    ratio: 0.1

  resource:
    attributes:
      - key: service.name
        value: Dunder Mifflin

    detectors:
      - os
      - process
  
exporters:
  otlp:
    endpoint: creedthoughts.gov

providers:
  tracer:
    processors: [sampler, resource]
    exporters: [otlp]
```

You can:
```go
package main

import (
	"context"

	"github.com/lesomnus/mkot"
	_ "github.com/lesomnus/mkot/otlp"
)

func main() {
	// Load expands ${env:VAR} references (like the collector) before parsing;
	// plain yaml.Unmarshal into a *mkot.Config also works, without expansion.
	conf, err := mkot.Load([]byte("..."))
	if err != nil {
		panic(err)
	}

	ctx := context.TODO()
	resolver := mkot.Make(ctx, conf)
	defer resolver.Shutdown(ctx)

	tracer_provider, err := resolver.Tracer(ctx, "")
	if err != nil {
		panic(err)
	}

	// Starts exporters.
	if err := resolver.Start(ctx); err != nil {
		panic(err)
	}

	_, span := tracer_provider.Tracer("dunder-mifflin").Start(ctx, "work")
	defer span.End()

	// ...
}
```

## otx

[otx][otx] carries the three providers through a `context.Context` instead of
threading them through every signature. `mkotx` builds one from a config, which
is the whole of the wiring above:

```go
package main

import (
	"context"

	"github.com/lesomnus/mkot"
	"github.com/lesomnus/mkot/mkotx"
	_ "github.com/lesomnus/mkot/otlp"
	"github.com/lesomnus/otx"
	"github.com/lesomnus/otx/log"
)

func main() {
	conf, err := mkot.Load([]byte("..."))
	if err != nil {
		panic(err)
	}

	ctx := context.TODO()

	// "" is the provider name, as in resolver.Tracer(ctx, "").
	x, err := mkotx.FromConfig(ctx, conf, "")
	if err != nil {
		panic(err)
	}

	// The resolver is the Controller of the Otx, so this starts the exporters
	// the config built, and Shutdown stops them.
	if err := x.Start(ctx); err != nil {
		panic(err)
	}
	defer x.Shutdown(ctx)

	ctx = otx.Into(ctx, x)

	// Anywhere downstream, with nothing but the context.
	ctx, span := otx.TraceStart(ctx, "work")
	defer span.End()

	log.From(ctx).Info("hello")
}
```

A signal the config does not mention resolves to `ErrNotExist`, which is not an
error here: that signal is simply off, and the no-op provider is used. Anything
else is returned, and no `Otx` with it. Take `mkotx.New` instead of
`FromConfig` when the resolver is needed afterwards, to resolve a second
provider under another name.

```sh
go get github.com/lesomnus/mkot/mkotx
```

It is a module of its own: otx is of no use to a program that only reads a
config, and mkot is of no use to one that is handed its providers by something
else.

[otx]: https://github.com/lesomnus/otx

## OTLP exporter

The `otlp` exporter wraps the OpenTelemetry Go SDK OTLP exporters. Its config
mirrors the opentelemetry-collector schema wherever the SDK can honor it.

```yaml
exporters:
  otlp:
    protocol: grpc            # grpc (default) or http/protobuf
    endpoint: collector:4317  # host:port, or a URL with scheme (http:// ⇒ insecure)
                              # grpc also accepts a gRPC target: dns:///, unix:///, xds:///
                              # under http it is a base URL: /v1/{traces,metrics,logs}
                              # is appended, so "https://host/otlp" posts to
                              # "https://host/otlp/v1/traces"
    compression: gzip         # gzip or none (only gzip is registered by the SDK);
                              # to guarantee none, leave OTEL_EXPORTER_OTLP_COMPRESSION unset
    timeout: 10s              # per-export deadline; above 30s applies to metrics
                              # only (the trace/log batchers cap exports at 30s)
    tls:
      insecure: false
      ca_file: /etc/otel/ca.pem
      min_version: "1.3"      # min/max_version and curve_preferences honored;
                              # cipher_suites applies to TLS 1.2 only (Go does not
                              # allow TLS 1.3 suite selection), and known-insecure
                              # suites are rejected
      cert_file: /etc/otel/client.pem  # mTLS; required by reload_interval
      key_file: /etc/otel/client.key
      reload_interval: 1h     # re-reads cert_file/key_file for mTLS rotation
    headers:
      - { name: authorization, value: "Bearer ..." }
    retry_on_failure:
      initial_interval: 5s
      max_interval: 30s
      max_elapsed_time: 1m    # 0 ⇒ never stop (differs from the collector's 5m default)
    sending_queue:            # applies to traces and logs (SDK batch processor)
      queue_size: 2048        # counted in spans/records, not bytes
      block_on_overflow: true # traces only; rejected on the log path
      batch:
        flush_timeout: 1s
        max_size: 512
    interval: 60s             # metric push period
    temporality: cumulative   # cumulative (default), delta, or lowmemory
    exemplar_filter: trace_based
    histogram_aggregation: explicit  # explicit (default) or exponential
```

Head sampling is a separate `sampler` processor:

```yaml
processors:
  sampler:
    type: trace_id_ratio      # always_on | always_off | trace_id_ratio | parent_based
    ratio: 0.1                # required by trace_id_ratio; optional for parent_based,
                              # which otherwise keeps every trace it roots
```

### Not supported

For the full collector-config parity map — what transfers directly, what
diverges, and the roadmap for the rest — see
[docs/collector-parity.md](docs/collector-parity.md).

Config the SDK cannot express is rejected with an error on the signals it
applies to, rather than silently dropped. These collector features have no
OpenTelemetry Go SDK equivalent and are not implemented:

- **`sending_queue`**: `num_consumers` above 1, `wait_for_result`,
  `batch.min_size`, and a persistent `storage` queue — the SDK batch processors
  cannot express them. `block_on_overflow` is honored for traces only and
  rejected for logs, whose SDK batch processor always drops on overflow.
  `sending_queue` governs traces/logs only; on a metric-only exporter it is
  ignored (not validated) — metric cadence is the `interval`.
- **`retry_on_failure`**: `randomization_factor` and `multiplier` — the SDK's
  backoff factors are fixed.
- **`protocol: http/protobuf`**: the gRPC-only knobs (`keepalive`,
  `read_buffer_size`, `write_buffer_size`, `wait_for_ready`, `balancer_name`,
  `authority`, `reconnection_period`) are rejected rather than ignored.
- **Metrics**: views (histogram bucket boundaries, instrument rename/drop,
  attribute/cardinality limits), a custom aggregation selector, and external
  producers are not exposed.
- **TLS**: TPM-backed keys.
- **gRPC**: reusing a pre-built connection / attaching interceptors (not
  expressible in YAML).

Auth (`auth:` block — `bearer` with a static token or a re-read `token_file`,
`basic`, or `oauth2` client-credentials) is configured inline rather than as a
named extension:

```yaml
exporters:
  otlp:
    auth:
      oauth2:
        client_id: ${env:CLIENT_ID}
        client_secret: ${env:CLIENT_SECRET}
        token_url: https://idp.example/oauth2/token
        scopes: [otlp.write]
```
