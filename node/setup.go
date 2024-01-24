package node

import (
	"time"

	cmcfg "github.com/cometbft/cometbft/config"
	proxy "github.com/cometbft/cometbft/proxy"

	"github.com/rollkit/rollkit/block"
	"github.com/rollkit/rollkit/mempool"
	"github.com/rollkit/rollkit/p2p"
	"github.com/rollkit/rollkit/state"
)

const readHeaderTimeout = 10 * time.Second

// MetricsProvider returns a consensus, p2p and mempool Metrics.
type MetricsProvider func(chainID string) (*block.Metrics, *p2p.Metrics, *mempool.Metrics, *state.Metrics, *proxy.Metrics)

// DefaultMetricsProvider returns Metrics build using Prometheus client library
// if Prometheus is enabled. Otherwise, it returns no-op Metrics.
func DefaultMetricsProvider(config *cmcfg.InstrumentationConfig) MetricsProvider {
	return func(chainID string) (*block.Metrics, *p2p.Metrics, *mempool.Metrics, *state.Metrics, *proxy.Metrics) {
		if config.Prometheus {
			return block.PrometheusMetrics(config.Namespace, "chain_id", chainID),
				p2p.PrometheusMetrics(config.Namespace, "chain_id", chainID),
				mempool.PrometheusMetrics(config.Namespace, "chain_id", chainID),
				state.PrometheusMetrics(config.Namespace, "chain_id", chainID),
				proxy.PrometheusMetrics(config.Namespace, "chain_id", chainID)
		}
		return block.NopMetrics(), p2p.NopMetrics(), mempool.NopMetrics(), state.NopMetrics(), proxy.NopMetrics()
	}
}
