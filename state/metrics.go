package state

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	"github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

const (
	// MetricsSubsystem is a subsystem shared by all metrics exposed by this
	// package.
	MetricsSubsystem = "state"
)

// Metrics contains metrics exposed by this package.
type Metrics struct {
	// Time spent processing FinalizeBlock
	BlockProcessingTime metrics.Histogram `metrics_buckettype:"lin" metrics_bucketsizes:"1, 10, 10"`

	// ConsensusParamUpdates is the total number of times the application has
	// updated the consensus params since process start.
	//metrics:Number of consensus parameter updates returned by the application since process start.
	ConsensusParamUpdates metrics.Counter
}

// PrometheusMetrics returns Metrics build using Prometheus client library.
// Optionally, labels can be provided along with their values ("foo",
// "fooValue").
func PrometheusMetrics(namespace string, labelsAndValues ...string) *Metrics {
	labels := []string{}
	for i := 0; i < len(labelsAndValues); i += 2 {
		labels = append(labels, labelsAndValues[i])
	}
	return &Metrics{
		BlockProcessingTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "block_processing_time",
			Help:      "Time spent processing FinalizeBlock",

			Buckets: stdprometheus.LinearBuckets(1, 10, 10),
		}, labels).With(labelsAndValues...),
		ConsensusParamUpdates: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "consensus_param_updates",
			Help:      "Number of consensus parameter updates returned by the application since process start.",
		}, labels).With(labelsAndValues...),
	}
}

// NopMetrics returns no-op Metrics.
func NopMetrics() *Metrics {
	return &Metrics{
		BlockProcessingTime:   discard.NewHistogram(),
		ConsensusParamUpdates: discard.NewCounter(),
	}
}
