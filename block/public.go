package block

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/evstack/ev-node/block/internal/common"
	"github.com/evstack/ev-node/block/internal/da"
	"github.com/evstack/ev-node/pkg/config"
	blobrpc "github.com/evstack/ev-node/pkg/da/jsonrpc"
)

// BlockOptions defines the options for creating block components
type BlockOptions = common.BlockOptions

// DefaultBlockOptions returns the default block options
func DefaultBlockOptions() BlockOptions {
	return common.DefaultBlockOptions()
}

// Expose the Metrics type for constructors
type Metrics = common.Metrics

// PrometheusMetrics creates a new PrometheusMetrics instance with the given namespace and labelsAndValues.
func PrometheusMetrics(namespace string, labelsAndValues ...string) *Metrics {
	return common.PrometheusMetrics(namespace, labelsAndValues...)
}

// NopMetrics creates a new NopMetrics instance.
func NopMetrics() *Metrics {
	return common.NopMetrics()
}

// DAClient is the interface representing the DA client for public use.
type DAClient = da.Client

// DAVerifier is the interface for DA proof verification operations.
type DAVerifier = da.Verifier

// FullDAClient combines DAClient and DAVerifier interfaces.
// This is the complete interface implemented by the concrete DA client.
type FullDAClient = da.FullClient

// BlobSubmitter submits blobs to DA and returns the inclusion height.
type BlobSubmitter = da.BlobSubmitter

// DAClientOption configures NewDAClient.
type DAClientOption func(*da.Config)

// WithSubmitter routes blob submission through s instead of the blob JSON-RPC
// API, e.g. to sign PayForBlobs locally. Reads still use the JSON-RPC client.
func WithSubmitter(s BlobSubmitter) DAClientOption {
	return func(c *da.Config) { c.Submitter = s }
}

// NewDAClient creates a new DA client backed by the blob JSON-RPC API.
// The returned client implements both DAClient and DAVerifier interfaces.
func NewDAClient(
	blobRPC *blobrpc.Client,
	config config.Config,
	logger zerolog.Logger,
	opts ...DAClientOption,
) FullDAClient {
	cfg := da.Config{
		DA:                       blobRPC,
		Logger:                   logger,
		Namespace:                config.DA.GetNamespace(),
		DefaultTimeout:           config.DA.RequestTimeout.Duration,
		DataNamespace:            config.DA.GetDataNamespace(),
		ForcedInclusionNamespace: config.DA.GetForcedInclusionNamespace(),
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	base := da.NewClient(cfg)
	if config.Instrumentation.IsTracingEnabled() {
		return da.WithTracingClient(base)
	}
	return base
}

// Exported errors used by the sequencers
var (
	// ErrForceInclusionNotConfigured is returned when force inclusion is not configured.
	ErrForceInclusionNotConfigured = da.ErrForceInclusionNotConfigured
	// ErrNoBatch is returned when a sequencer does not have a batch to return.
	ErrNoBatch = common.ErrNoBatch
)

// ForcedInclusionEvent represents forced inclusion transactions retrieved from DA
type ForcedInclusionEvent = da.ForcedInclusionEvent

// ForcedInclusionRetriever defines the interface for retrieving forced inclusion transactions from DA
type ForcedInclusionRetriever interface {
	RetrieveForcedIncludedTxs(ctx context.Context, daHeight uint64) (*ForcedInclusionEvent, error)
	Stop()
	Start(ctx context.Context)
}

// NewForcedInclusionRetriever creates a new forced inclusion retriever.
// It internally creates and manages an AsyncBlockRetriever for background prefetching.
// Tracing is automatically enabled when configured.
func NewForcedInclusionRetriever(
	client DAClient,
	cfg config.Config,
	logger zerolog.Logger,
	daStartHeight, daEpochSize uint64,
) ForcedInclusionRetriever {
	return da.NewForcedInclusionRetriever(
		client,
		logger,
		cfg.DA.BlockTime.Duration,
		cfg.Instrumentation.IsTracingEnabled(),
		daStartHeight,
		daEpochSize,
	)
}

// Expose the Raft node type for consensus integration
type RaftNode = common.RaftNode
