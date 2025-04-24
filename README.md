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
