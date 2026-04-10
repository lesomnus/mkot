package mkot

import "time"

// TODO: see https://github.com/open-telemetry/opentelemetry-collector/blob/main/exporter/exporterhelper/internal/retry_sender.go

type RetryConfig struct {
	// Enabled indicates whether to not retry sending batches in case of export failure.
	Enabled *bool
	// InitialInterval the time to wait after the first failure before retrying.
	InitialInterval time.Duration `yaml:"initial_interval"`
	// RandomizationFactor is a random factor used to calculate next backoffs
	// Randomized interval = RetryInterval * (1 ± RandomizationFactor)
	RandomizationFactor float64 `yaml:"randomization_factor"`
	// Multiplier is the value multiplied by the backoff interval bounds
	Multiplier float64 `yaml:"multiplier"`
	// MaxInterval is the upper bound on backoff interval. Once this value is reached the delay between
	// consecutive retries will always be `MaxInterval`.
	MaxInterval time.Duration `yaml:"max_interval"`
	// MaxElapsedTime is the maximum amount of time (including retries) spent trying to send a request/batch.
	// Once this value is reached, the data is discarded. If set to 0, the retries are never stopped.
	MaxElapsedTime time.Duration `yaml:"max_elapsed_time"`
}

func (c RetryConfig) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}
