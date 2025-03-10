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

// Metrics contains metrics exposed by this package.
type Metrics struct {
	// Height of the chain.
	Height metrics.Gauge

	// Number of transactions.
	NumTxs metrics.Gauge
	// Size of the block.
	BlockSizeBytes metrics.Gauge
	// Total number of transactions.
	TotalTxs metrics.Gauge
	// The latest block height.
	CommittedHeight metrics.Gauge `metrics_name:"latest_block_height"`

	// Execution time
	ExecutionTime metrics.Histogram
	// Signature time
	SignatureTime metrics.Histogram
	// Commit time
	CommitTime metrics.Histogram
	// Save time
	SaveTime metrics.Histogram
	// Transaction count
	TxCount metrics.Histogram

	// HeaderInChanLen
	HeaderInChanLen metrics.Gauge
	// DataInChanLen
	DataInChanLen metrics.Gauge
	// PendingHeadersCount
	PendingHeadersCount metrics.Gauge
	// DAInclusionDelay
	DAInclusionDelay metrics.Histogram
	// SchedulerTxsTotal
	SchedulerTxsTotal metrics.Counter
	// SchedulerTxBytes
	SchedulerTxBytes metrics.Counter
	// SaveMetricsTime
	SaveMetricsTime metrics.Histogram

	// Batch processing metrics
	TxsPerBatch      metrics.Histogram
	TxBytesPerBatch  metrics.Histogram
	BatchSubmissions metrics.Counter
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
		Height: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "height",
			Help:      "Height of the chain.",
		}, labels).With(labelsAndValues...),
		NumTxs: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "num_txs",
			Help:      "Number of transactions.",
		}, labels).With(labelsAndValues...),
		BlockSizeBytes: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "block_size_bytes",
			Help:      "Size of the block.",
		}, labels).With(labelsAndValues...),
		TotalTxs: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "total_txs",
			Help:      "Total number of transactions.",
		}, labels).With(labelsAndValues...),
		CommittedHeight: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "latest_block_height",
			Help:      "The latest block height.",
		}, labels).With(labelsAndValues...),
		ExecutionTime: prometheus.NewHistogram(
			stdprometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: MetricsSubsystem,
				Name:      "execution_time_seconds",
				Help:      "Execution time of the block processing.",
			},
			labels,
		),
		SignatureTime: prometheus.NewHistogram(
			stdprometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: MetricsSubsystem,
				Name:      "signature_time_seconds",
				Help:      "Signature time of the block processing.",
			},
			labels,
		),
		CommitTime: prometheus.NewHistogram(
			stdprometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: MetricsSubsystem,
				Name:      "commit_time_seconds",
				Help:      "Commit time of the block processing.",
			},
			labels,
		),
		SaveTime: prometheus.NewHistogram(
			stdprometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: MetricsSubsystem,
				Name:      "save_time_seconds",
				Help:      "Save time of the block processing.",
			},
			labels,
		),
		TxCount: prometheus.NewHistogram(
			stdprometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: MetricsSubsystem,
				Name:      "tx_count",
				Help:      "Transaction count of the block processing.",
			},
			labels,
		),
		HeaderInChanLen: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "block_manager_header_in_chan_len",
			Help:      "Number of items in the HeaderCh of block manager",
		}, labels),
		DataInChanLen: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "block_manager_data_in_chan_len",
			Help:      "Number of items in the DataCh of block manager",
		}, labels),
		PendingHeadersCount: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "block_manager_pending_headers_count",
			Help:      "Number of pending headers in the block manager",
		}, labels),
		DAInclusionDelay: prometheus.NewHistogram(
			stdprometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: MetricsSubsystem,
				Name:      "block_manager_da_inclusion_delay_seconds",
				Help:      "DA inclusion delay of the block processing.",
			},
			labels,
		),
		SchedulerTxsTotal: prometheus.NewCounter(
			stdprometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: MetricsSubsystem,
				Name:      "block_manager_scheduler_txs_total",
				Help:      "Total number of transactions scheduled by the block manager",
			},
			labels,
		),
		SchedulerTxBytes: prometheus.NewCounter(
			stdprometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: MetricsSubsystem,
				Name:      "block_manager_scheduler_tx_bytes_total",
				Help:      "Total number of transaction bytes scheduled by the block manager",
			},
			labels,
		),
		SaveMetricsTime: prometheus.NewHistogram(
			stdprometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: MetricsSubsystem,
				Name:      "block_manager_save_metrics_time_seconds",
				Help:      "Time taken to save block metrics",
			},
			labels,
		),
		TxsPerBatch: prometheus.NewHistogram(
			stdprometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: MetricsSubsystem,
				Name:      "block_manager_txs_per_batch",
				Help:      "Number of transactions per batch",
			},
			labels,
		),
		TxBytesPerBatch: prometheus.NewHistogram(
			stdprometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: MetricsSubsystem,
				Name:      "block_manager_tx_bytes_per_batch",
				Help:      "Number of transaction bytes per batch",
			},
			labels,
		),
		BatchSubmissions: prometheus.NewCounter(
			stdprometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: MetricsSubsystem,
				Name:      "block_manager_batch_submissions_total",
				Help:      "Total number of batch submissions to DA",
			},
			labels,
		),
	}
}

// NopMetrics returns no-op Metrics.
func NopMetrics() *Metrics {
	return &Metrics{
		Height:              discard.NewGauge(),
		NumTxs:              discard.NewGauge(),
		BlockSizeBytes:      discard.NewGauge(),
		TotalTxs:            discard.NewGauge(),
		CommittedHeight:     discard.NewGauge(),
		ExecutionTime:       discard.NewHistogram(),
		SignatureTime:       discard.NewHistogram(),
		CommitTime:          discard.NewHistogram(),
		SaveTime:            discard.NewHistogram(),
		TxCount:             discard.NewHistogram(),
		HeaderInChanLen:     discard.NewGauge(),
		DataInChanLen:       discard.NewGauge(),
		PendingHeadersCount: discard.NewGauge(),
		DAInclusionDelay:    discard.NewHistogram(),
		SchedulerTxsTotal:   discard.NewCounter(),
		SchedulerTxBytes:    discard.NewCounter(),
		SaveMetricsTime:     discard.NewHistogram(),
		TxsPerBatch:         discard.NewHistogram(),
		TxBytesPerBatch:     discard.NewHistogram(),
		BatchSubmissions:    discard.NewCounter(),
	}
}
