package node

import (
	"time"

	cmcfg "github.com/cometbft/cometbft/config"

	"github.com/rollkit/rollkit/block"
	"github.com/rollkit/rollkit/p2p"
)

const readHeaderTimeout = 10 * time.Second

// MetricsProvider returns a consensus, p2p and mempool Metrics.
type MetricsProvider func(chainID string) (*block.Metrics, *p2p.Metrics)

// DefaultMetricsProvider returns Metrics build using Prometheus client library
// if Prometheus is enabled. Otherwise, it returns no-op Metrics.
func DefaultMetricsProvider(config *cmcfg.InstrumentationConfig) MetricsProvider {
	return func(chainID string) (*block.Metrics, *p2p.Metrics) {
		if config.Prometheus {
			return block.PrometheusMetrics(config.Namespace, "chain_id", chainID),
				p2p.PrometheusMetrics(config.Namespace, "chain_id", chainID),
		}
		return block.NopMetrics(), p2p.NopMetrics()
	}
}
