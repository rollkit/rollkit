package block

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	"github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

const (
	// MetricsSubsystem is a subsystem shared by all metrics exposed by this
	// package.
	MetricsSubsystem = "sequencer"
)

// Metrics contains all metrics exposed by this package.
type Metrics struct {
	// Original metrics
	Height          metrics.Gauge // Height of the chain
	NumTxs          metrics.Gauge // Number of transactions in the latest block
	BlockSizeBytes  metrics.Gauge // Size of the latest block
	TotalTxs        metrics.Gauge // Total number of transactions
	CommittedHeight metrics.Gauge `metrics_name:"latest_block_height"` // The latest block height

	// Channel metrics
	ChannelBufferUsage map[string]metrics.Gauge
	DroppedSignals     metrics.Counter

	// Error metrics
	ErrorsByType         map[string]metrics.Counter
	RecoverableErrors    metrics.Counter
	NonRecoverableErrors metrics.Counter

	// Performance metrics
	OperationDuration map[string]metrics.Histogram
	GoroutineCount    metrics.Gauge

	// DA metrics
	DASubmissionAttempts  metrics.Counter
	DASubmissionSuccesses metrics.Counter
	DASubmissionFailures  metrics.Counter
	DARetrievalAttempts   metrics.Counter
	DARetrievalSuccesses  metrics.Counter
	DARetrievalFailures   metrics.Counter
	DAInclusionHeight     metrics.Gauge
	PendingHeadersCount   metrics.Gauge
	PendingDataCount      metrics.Gauge

	// Sync metrics
	SyncLag             metrics.Gauge
	HeadersSynced       metrics.Counter
	DataSynced          metrics.Counter
	BlocksApplied       metrics.Counter
	InvalidHeadersCount metrics.Counter

	// Block production metrics
	BlockProductionTime  metrics.Histogram
	EmptyBlocksProduced  metrics.Counter
	LazyBlocksProduced   metrics.Counter
	NormalBlocksProduced metrics.Counter
	TxsPerBlock          metrics.Histogram

	// State transition metrics
	StateTransitions   map[string]metrics.Counter
	InvalidTransitions metrics.Counter
}

// PrometheusMetrics returns Metrics built using Prometheus client library
func PrometheusMetrics(namespace string, labelsAndValues ...string) *Metrics {
	labels := []string{}
	for i := 0; i < len(labelsAndValues); i += 2 {
		labels = append(labels, labelsAndValues[i])
	}

	m := &Metrics{
		ChannelBufferUsage: make(map[string]metrics.Gauge),
		ErrorsByType:       make(map[string]metrics.Counter),
		OperationDuration:  make(map[string]metrics.Histogram),
		StateTransitions:   make(map[string]metrics.Counter),
	}

	// Original metrics
	m.Height = prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "height",
		Help:      "Height of the chain.",
	}, labels).With(labelsAndValues...)

	m.NumTxs = prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "num_txs",
		Help:      "Number of transactions.",
	}, labels).With(labelsAndValues...)

	m.BlockSizeBytes = prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "block_size_bytes",
		Help:      "Size of the block.",
	}, labels).With(labelsAndValues...)

	m.TotalTxs = prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "total_txs",
		Help:      "Total number of transactions.",
	}, labels).With(labelsAndValues...)

	m.CommittedHeight = prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "latest_block_height",
		Help:      "The latest block height.",
	}, labels).With(labelsAndValues...)

	// Channel metrics
	m.DroppedSignals = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "dropped_signals_total",
		Help:      "Total number of dropped channel signals",
	}, labels).With(labelsAndValues...)

	// Initialize channel buffer usage gauges
	channelNames := []string{"header_in", "data_in", "header_store", "data_store", "retrieve", "da_includer", "tx_notify"}
	for _, name := range channelNames {
		m.ChannelBufferUsage[name] = prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "channel_buffer_usage",
			Help:      "Current buffer usage of channels",
			ConstLabels: map[string]string{
				"channel": name,
			},
		}, labels).With(labelsAndValues...)
	}

	// Error metrics
	m.RecoverableErrors = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "recoverable_errors_total",
		Help:      "Total number of recoverable errors",
	}, labels).With(labelsAndValues...)

	m.NonRecoverableErrors = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "non_recoverable_errors_total",
		Help:      "Total number of non-recoverable errors",
	}, labels).With(labelsAndValues...)

	// Initialize error type counters
	errorTypes := []string{"block_production", "da_submission", "sync", "validation", "state_update"}
	for _, errType := range errorTypes {
		m.ErrorsByType[errType] = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "errors_by_type_total",
			Help:      "Total number of errors by type",
			ConstLabels: map[string]string{
				"error_type": errType,
			},
		}, labels).With(labelsAndValues...)
	}

	// Performance metrics
	m.GoroutineCount = prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "goroutines_count",
		Help:      "Current number of goroutines",
	}, labels).With(labelsAndValues...)

	// Initialize operation duration histograms
	operations := []string{"block_production", "da_submission", "block_retrieval", "block_validation", "state_update"}
	for _, op := range operations {
		m.OperationDuration[op] = prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "operation_duration_seconds",
			Help:      "Duration of operations in seconds",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			ConstLabels: map[string]string{
				"operation": op,
			},
		}, labels).With(labelsAndValues...)
	}

	// DA metrics
	m.DASubmissionAttempts = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "da_submission_attempts_total",
		Help:      "Total number of DA submission attempts",
	}, labels).With(labelsAndValues...)

	m.DASubmissionSuccesses = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "da_submission_successes_total",
		Help:      "Total number of successful DA submissions",
	}, labels).With(labelsAndValues...)

	m.DASubmissionFailures = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "da_submission_failures_total",
		Help:      "Total number of failed DA submissions",
	}, labels).With(labelsAndValues...)

	m.DARetrievalAttempts = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "da_retrieval_attempts_total",
		Help:      "Total number of DA retrieval attempts",
	}, labels).With(labelsAndValues...)

	m.DARetrievalSuccesses = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "da_retrieval_successes_total",
		Help:      "Total number of successful DA retrievals",
	}, labels).With(labelsAndValues...)

	m.DARetrievalFailures = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "da_retrieval_failures_total",
		Help:      "Total number of failed DA retrievals",
	}, labels).With(labelsAndValues...)

	m.DAInclusionHeight = prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "da_inclusion_height",
		Help:      "Height at which all blocks have been included in DA",
	}, labels).With(labelsAndValues...)

	m.PendingHeadersCount = prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "pending_headers_count",
		Help:      "Number of headers pending DA submission",
	}, labels).With(labelsAndValues...)

	m.PendingDataCount = prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "pending_data_count",
		Help:      "Number of data blocks pending DA submission",
	}, labels).With(labelsAndValues...)

	// Sync metrics
	m.SyncLag = prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "sync_lag_blocks",
		Help:      "Number of blocks behind the head",
	}, labels).With(labelsAndValues...)

	m.HeadersSynced = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "headers_synced_total",
		Help:      "Total number of headers synced",
	}, labels).With(labelsAndValues...)

	m.DataSynced = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "data_synced_total",
		Help:      "Total number of data blocks synced",
	}, labels).With(labelsAndValues...)

	m.BlocksApplied = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "blocks_applied_total",
		Help:      "Total number of blocks applied to state",
	}, labels).With(labelsAndValues...)

	m.InvalidHeadersCount = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "invalid_headers_total",
		Help:      "Total number of invalid headers rejected",
	}, labels).With(labelsAndValues...)

	// Block production metrics
	m.BlockProductionTime = prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "block_production_duration_seconds",
		Help:      "Time taken to produce a block",
		Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
	}, labels).With(labelsAndValues...)

	m.EmptyBlocksProduced = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "empty_blocks_produced_total",
		Help:      "Total number of empty blocks produced",
	}, labels).With(labelsAndValues...)

	m.LazyBlocksProduced = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "lazy_blocks_produced_total",
		Help:      "Total number of blocks produced in lazy mode",
	}, labels).With(labelsAndValues...)

	m.NormalBlocksProduced = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "normal_blocks_produced_total",
		Help:      "Total number of blocks produced in normal mode",
	}, labels).With(labelsAndValues...)

	m.TxsPerBlock = prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "txs_per_block",
		Help:      "Number of transactions per block",
		Buckets:   []float64{0, 1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000},
	}, labels).With(labelsAndValues...)

	// State transition metrics
	m.InvalidTransitions = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: MetricsSubsystem,
		Name:      "invalid_state_transitions_total",
		Help:      "Total number of invalid state transitions attempted",
	}, labels).With(labelsAndValues...)

	// Initialize state transition counters
	transitions := []string{"pending_to_submitted", "submitted_to_included", "included_to_finalized"}
	for _, transition := range transitions {
		m.StateTransitions[transition] = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "state_transitions_total",
			Help:      "Total number of state transitions",
			ConstLabels: map[string]string{
				"transition": transition,
			},
		}, labels).With(labelsAndValues...)
	}

	return m
}

// NopMetrics returns no-op Metrics
func NopMetrics() *Metrics {
	m := &Metrics{
		// Original metrics
		Height:          discard.NewGauge(),
		NumTxs:          discard.NewGauge(),
		BlockSizeBytes:  discard.NewGauge(),
		TotalTxs:        discard.NewGauge(),
		CommittedHeight: discard.NewGauge(),

		// Extended metrics
		ChannelBufferUsage:    make(map[string]metrics.Gauge),
		ErrorsByType:          make(map[string]metrics.Counter),
		OperationDuration:     make(map[string]metrics.Histogram),
		StateTransitions:      make(map[string]metrics.Counter),
		DroppedSignals:        discard.NewCounter(),
		RecoverableErrors:     discard.NewCounter(),
		NonRecoverableErrors:  discard.NewCounter(),
		GoroutineCount:        discard.NewGauge(),
		DASubmissionAttempts:  discard.NewCounter(),
		DASubmissionSuccesses: discard.NewCounter(),
		DASubmissionFailures:  discard.NewCounter(),
		DARetrievalAttempts:   discard.NewCounter(),
		DARetrievalSuccesses:  discard.NewCounter(),
		DARetrievalFailures:   discard.NewCounter(),
		DAInclusionHeight:     discard.NewGauge(),
		PendingHeadersCount:   discard.NewGauge(),
		PendingDataCount:      discard.NewGauge(),
		SyncLag:               discard.NewGauge(),
		HeadersSynced:         discard.NewCounter(),
		DataSynced:            discard.NewCounter(),
		BlocksApplied:         discard.NewCounter(),
		InvalidHeadersCount:   discard.NewCounter(),
		BlockProductionTime:   discard.NewHistogram(),
		EmptyBlocksProduced:   discard.NewCounter(),
		LazyBlocksProduced:    discard.NewCounter(),
		NormalBlocksProduced:  discard.NewCounter(),
		TxsPerBlock:           discard.NewHistogram(),
		InvalidTransitions:    discard.NewCounter(),
	}

	// Initialize maps with no-op metrics
	channelNames := []string{"header_in", "data_in", "header_store", "data_store", "retrieve", "da_includer", "tx_notify"}
	for _, name := range channelNames {
		m.ChannelBufferUsage[name] = discard.NewGauge()
	}

	errorTypes := []string{"block_production", "da_submission", "sync", "validation", "state_update"}
	for _, errType := range errorTypes {
		m.ErrorsByType[errType] = discard.NewCounter()
	}

	operations := []string{"block_production", "da_submission", "block_retrieval", "block_validation", "state_update"}
	for _, op := range operations {
		m.OperationDuration[op] = discard.NewHistogram()
	}

	transitions := []string{"pending_to_submitted", "submitted_to_included", "included_to_finalized"}
	for _, transition := range transitions {
		m.StateTransitions[transition] = discard.NewCounter()
	}

	return m
}
