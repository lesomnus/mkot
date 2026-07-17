# mkot

*mkot* is a utility designed to simplify the creation of OpenTelemetry Providers.
It supports configuration patterns that closely align with those used in opentelemetry-collector, enabling seamless integration and familiar setup for observability pipelines.

## Peek

From:
```yaml
enabled: true
processors:
  batcher/foo:
    max_queue_size: 42

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
    processors: [batcher/foo, resource]
    exporters: [otlp]
```

You can:
```go
package main

import (
	"context"

	"github.com/lesomnus/mkot"
	_ "github.com/lesomnus/mkot/otlp"
	"gopkg.in/yaml.v3"
)

func main() {
	conf := &mkot.Config{}
	err := yaml.Unmarshal([]byte("..."), conf)
	if err != nil {
		panic(err)
	}

	ctx := context.TODO()
	resolver := mkot.Make(conf)
	defer resolver.Shutdown(ctx)

	tracer_provider, err := resolver.Tracer(ctx, "")
	if err != nil {
		panic(err)
	}

	// Starts exporters.
	if err := resolver.Start(ctx); err != nil {
		panic(err)
	}

	// ...
}
```

## OTLP exporter

The `otlp` exporter wraps the OpenTelemetry Go SDK OTLP exporters. Its config
mirrors the opentelemetry-collector schema wherever the SDK can honor it.

```yaml
exporters:
  otlp:
    protocol: grpc            # grpc (default) or http/protobuf
    endpoint: collector:4317  # host:port, or a URL with scheme (http:// ⇒ insecure)
    compression: gzip         # gzip or none (only gzip is registered by the SDK)
    timeout: 10s              # per-export deadline
    tls:
      insecure: false
      ca_file: /etc/otel/ca.pem
      min_version: "1.3"      # min/max_version, cipher_suites, curve_preferences honored
      reload_interval: 1h     # reloads cert_file/key_file for mTLS rotation
    headers:
      - { name: authorization, value: "Bearer ..." }
    retry_on_failure:
      initial_interval: 5s
      max_interval: 30s
      max_elapsed_time: 1m    # 0 ⇒ never stop (differs from the collector's 5m default)
    sending_queue:            # applies to traces and logs (SDK batch processor)
      queue_size: 2048        # counted in spans/records, not bytes
      block_on_overflow: true # spans block instead of dropping
      batch:
        flush_timeout: 1s
        max_size: 512
    interval: 60s             # metric push period
    temporality: cumulative   # cumulative (default), delta, or lowmemory
    exemplar_filter: trace_based
```

Head sampling is a separate `sampler` processor:

```yaml
processors:
  sampler:
    type: trace_id_ratio      # always_on | always_off | trace_id_ratio | parent_based
    ratio: 0.1
```

### Not supported

Config the SDK cannot express is rejected with an error rather than silently
dropped. These collector features have no OpenTelemetry Go SDK equivalent and
are not implemented:

- **`sending_queue`**: `num_consumers`, `wait_for_result`, `batch.min_size`, and
  a persistent `storage` queue — the SDK batch processors cannot express them.
  `sending_queue` governs traces/logs only; metric cadence is the `interval`.
- **Auth**: only static `headers` (e.g. a fixed bearer token). OAuth2 or
  refreshing-token auth extensions are not available — build the provider by hand
  for those.
- **Metrics**: views (histogram bucket boundaries, instrument rename/drop,
  attribute/cardinality limits), a custom aggregation selector, and external
  producers are not exposed.
- **TLS**: TPM-backed keys.
- **gRPC**: a custom service config beyond `balancer_name`, or reusing a pre-built
  connection / attaching interceptors (not expressible in YAML).
