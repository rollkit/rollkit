package config

import (
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	// FlagRootDir is a flag for specifying the root directory
	FlagRootDir = "home"
	// FlagAggregator is a flag for running node in aggregator mode
	FlagAggregator = "rollkit.aggregator"
	// FlagDAAddress is a flag for specifying the data availability layer address
	FlagDAAddress = "rollkit.da_address"
	// FlagDAAuthToken is a flag for specifying the data availability layer auth token
	FlagDAAuthToken = "rollkit.da_auth_token" // #nosec G101
	// FlagBlockTime is a flag for specifying the block time
	FlagBlockTime = "rollkit.block_time"
	// FlagDABlockTime is a flag for specifying the data availability layer block time
	FlagDABlockTime = "rollkit.da_block_time"
	// FlagDAGasPrice is a flag for specifying the data availability layer gas price
	FlagDAGasPrice = "rollkit.da_gas_price"
	// FlagDAGasMultiplier is a flag for specifying the data availability layer gas price retry multiplier
	FlagDAGasMultiplier = "rollkit.da_gas_multiplier"
	// FlagDAStartHeight is a flag for specifying the data availability layer start height
	FlagDAStartHeight = "rollkit.da_start_height"
	// FlagDANamespace is a flag for specifying the DA namespace ID
	FlagDANamespace = "rollkit.da_namespace"
	// FlagDASubmitOptions is a flag for data availability submit options
	FlagDASubmitOptions = "rollkit.da_submit_options"
	// FlagLight is a flag for running the node in light mode
	FlagLight = "rollkit.light"
	// FlagTrustedHash is a flag for specifying the trusted hash
	FlagTrustedHash = "rollkit.trusted_hash"
	// FlagLazyAggregator is a flag for enabling lazy aggregation
	FlagLazyAggregator = "rollkit.lazy_aggregator"
	// FlagMaxPendingBlocks is a flag to pause aggregator in case of large number of blocks pending DA submission
	FlagMaxPendingBlocks = "rollkit.max_pending_blocks"
	// FlagDAMempoolTTL is a flag for specifying the DA mempool TTL
	FlagDAMempoolTTL = "rollkit.da_mempool_ttl"
	// FlagLazyBlockTime is a flag for specifying the block time in lazy mode
	FlagLazyBlockTime = "rollkit.lazy_block_time"
	// FlagSequencerAddress is a flag for specifying the sequencer middleware address
	FlagSequencerAddress = "rollkit.sequencer_address"
	// FlagSequencerRollupID is a flag for specifying the sequencer middleware rollup ID
	FlagSequencerRollupID = "rollkit.sequencer_rollup_id"
	// FlagExecutorAddress is a flag for specifying the sequencer middleware address
	FlagExecutorAddress = "rollkit.executor_address"
	// FlagDBPath is a flag for specifying the database path
	FlagDBPath = "rollkit.db_path"
	// FlagPrometheus is a flag for enabling Prometheus metrics
	FlagPrometheus = "rollkit.instrumentation.prometheus"
	// FlagPrometheusListenAddr is a flag for specifying the Prometheus listen address
	FlagPrometheusListenAddr = "rollkit.instrumentation.prometheus_listen_addr"
	// FlagMaxOpenConnections is a flag for specifying the maximum number of open connections
	FlagMaxOpenConnections = "rollkit.instrumentation.max_open_connections"
)

// NodeConfig stores Rollkit node configuration.
type NodeConfig struct {
	// parameters below are translated from existing config
	RootDir string    `mapstructure:"home"`
	P2P     P2PConfig `mapstructure:"p2p"`

	// Rollkit specific configuration
	Rollkit         RollkitConfig          `mapstructure:"rollkit"`
	Instrumentation *InstrumentationConfig `mapstructure:"instrumentation"`
}

// RollkitConfig contains all Rollkit specific configuration parameters
type RollkitConfig struct {
	// Database configuration
	DBPath string `mapstructure:"db_path"`

	// Node mode configuration
	Aggregator bool `mapstructure:"aggregator"`
	Light      bool `mapstructure:"light"`

	// Data availability configuration
	DAAddress       string  `mapstructure:"da_address"`
	DAAuthToken     string  `mapstructure:"da_auth_token"`
	DAGasPrice      float64 `mapstructure:"da_gas_price"`
	DAGasMultiplier float64 `mapstructure:"da_gas_multiplier"`
	DASubmitOptions string  `mapstructure:"da_submit_options"`
	DANamespace     string  `mapstructure:"da_namespace"`

	// Block management configuration
	BlockTime        time.Duration `mapstructure:"block_time"`
	DABlockTime      time.Duration `mapstructure:"da_block_time"`
	DAStartHeight    uint64        `mapstructure:"da_start_height"`
	DAMempoolTTL     uint64        `mapstructure:"da_mempool_ttl"`
	MaxPendingBlocks uint64        `mapstructure:"max_pending_blocks"`
	LazyAggregator   bool          `mapstructure:"lazy_aggregator"`
	LazyBlockTime    time.Duration `mapstructure:"lazy_block_time"`

	// Header configuration
	TrustedHash string `mapstructure:"trusted_hash"`

	// Sequencer configuration
	SequencerAddress  string `mapstructure:"sequencer_address"`
	SequencerRollupID string `mapstructure:"sequencer_rollup_id"`
	ExecutorAddress   string `mapstructure:"executor_address"`
}

// GetViperConfig reads configuration parameters from Viper instance.
//
// This method is called in cosmos-sdk.
func (nc *NodeConfig) GetViperConfig(v *viper.Viper) error {
	if v.IsSet("root_dir") {
		nc.RootDir = v.GetString("root_dir")
	}
	if v.IsSet(FlagDBPath) {
		nc.Rollkit.DBPath = v.GetString(FlagDBPath)
	}

	if v.IsSet("p2p.laddr") {
		nc.P2P.ListenAddress = v.GetString("p2p.laddr")
	}
	if v.IsSet("p2p.seeds") {
		nc.P2P.Seeds = v.GetString("p2p.seeds")
	}
	if v.IsSet("p2p.blocked_peers") {
		nc.P2P.BlockedPeers = v.GetString("p2p.blocked_peers")
	}
	if v.IsSet("p2p.allowed_peers") {
		nc.P2P.AllowedPeers = v.GetString("p2p.allowed_peers")
	}

	if v.IsSet("instrumentation") {
		if nc.Instrumentation == nil {
			nc.Instrumentation = &InstrumentationConfig{}
		}
		if v.IsSet("instrumentation.prometheus") {
			nc.Instrumentation.Prometheus = v.GetBool("instrumentation.prometheus")
		}
		if v.IsSet("instrumentation.prometheus_listen_addr") {
			nc.Instrumentation.PrometheusListenAddr = v.GetString("instrumentation.prometheus_listen_addr")
		}
		if v.IsSet("instrumentation.max_open_connections") {
			nc.Instrumentation.MaxOpenConnections = v.GetInt("instrumentation.max_open_connections")
		}
		nc.Instrumentation.Namespace = "rollkit"
	}

	// Check for the new Instrumentation flags
	if v.IsSet(FlagPrometheus) || v.IsSet(FlagPrometheusListenAddr) || v.IsSet(FlagMaxOpenConnections) {
		if nc.Instrumentation == nil {
			nc.Instrumentation = &InstrumentationConfig{}
			nc.Instrumentation.Namespace = "rollkit"
		}
		if v.IsSet(FlagPrometheus) {
			nc.Instrumentation.Prometheus = v.GetBool(FlagPrometheus)
		}
		if v.IsSet(FlagPrometheusListenAddr) {
			nc.Instrumentation.PrometheusListenAddr = v.GetString(FlagPrometheusListenAddr)
		}
		if v.IsSet(FlagMaxOpenConnections) {
			nc.Instrumentation.MaxOpenConnections = v.GetInt(FlagMaxOpenConnections)
		}
	}

	if v.IsSet(FlagAggregator) {
		nc.Rollkit.Aggregator = v.GetBool(FlagAggregator)
	}
	if v.IsSet(FlagDAAddress) {
		nc.Rollkit.DAAddress = v.GetString(FlagDAAddress)
	}
	if v.IsSet(FlagDAAuthToken) {
		nc.Rollkit.DAAuthToken = v.GetString(FlagDAAuthToken)
	}
	if v.IsSet(FlagDAGasPrice) {
		nc.Rollkit.DAGasPrice = v.GetFloat64(FlagDAGasPrice)
	}
	if v.IsSet(FlagDAGasMultiplier) {
		nc.Rollkit.DAGasMultiplier = v.GetFloat64(FlagDAGasMultiplier)
	}
	if v.IsSet(FlagDANamespace) {
		nc.Rollkit.DANamespace = v.GetString(FlagDANamespace)
	}
	if v.IsSet(FlagDAStartHeight) {
		nc.Rollkit.DAStartHeight = v.GetUint64(FlagDAStartHeight)
	}
	if v.IsSet(FlagDABlockTime) {
		nc.Rollkit.DABlockTime = v.GetDuration(FlagDABlockTime)
	}
	if v.IsSet(FlagDASubmitOptions) {
		nc.Rollkit.DASubmitOptions = v.GetString(FlagDASubmitOptions)
	}
	if v.IsSet(FlagBlockTime) {
		nc.Rollkit.BlockTime = v.GetDuration(FlagBlockTime)
	}
	if v.IsSet(FlagLazyAggregator) {
		nc.Rollkit.LazyAggregator = v.GetBool(FlagLazyAggregator)
	}
	if v.IsSet(FlagLight) {
		nc.Rollkit.Light = v.GetBool(FlagLight)
	}
	if v.IsSet(FlagTrustedHash) {
		nc.Rollkit.TrustedHash = v.GetString(FlagTrustedHash)
	}
	if v.IsSet(FlagMaxPendingBlocks) {
		nc.Rollkit.MaxPendingBlocks = v.GetUint64(FlagMaxPendingBlocks)
	}
	if v.IsSet(FlagDAMempoolTTL) {
		nc.Rollkit.DAMempoolTTL = v.GetUint64(FlagDAMempoolTTL)
	}
	if v.IsSet(FlagLazyBlockTime) {
		nc.Rollkit.LazyBlockTime = v.GetDuration(FlagLazyBlockTime)
	}
	if v.IsSet(FlagSequencerAddress) {
		nc.Rollkit.SequencerAddress = v.GetString(FlagSequencerAddress)
	}
	if v.IsSet(FlagSequencerRollupID) {
		nc.Rollkit.SequencerRollupID = v.GetString(FlagSequencerRollupID)
	}
	if v.IsSet(FlagExecutorAddress) {
		nc.Rollkit.ExecutorAddress = v.GetString(FlagExecutorAddress)
	}

	return nil
}

// AddFlags adds Rollkit specific configuration options to cobra Command.
//
// This function is called in cosmos-sdk.
func AddFlags(cmd *cobra.Command) {
	def := DefaultNodeConfig

	cmd.Flags().BoolVar(&def.Rollkit.Aggregator, FlagAggregator, def.Rollkit.Aggregator, "run node in aggregator mode")
	cmd.Flags().Bool(FlagLazyAggregator, def.Rollkit.LazyAggregator, "wait for transactions, don't build empty blocks")
	cmd.Flags().String(FlagDAAddress, def.Rollkit.DAAddress, "DA address (host:port)")
	cmd.Flags().String(FlagDAAuthToken, def.Rollkit.DAAuthToken, "DA auth token")
	cmd.Flags().Duration(FlagBlockTime, def.Rollkit.BlockTime, "block time (for aggregator mode)")
	cmd.Flags().Duration(FlagDABlockTime, def.Rollkit.DABlockTime, "DA chain block time (for syncing)")
	cmd.Flags().Float64(FlagDAGasPrice, def.Rollkit.DAGasPrice, "DA gas price for blob transactions")
	cmd.Flags().Float64(FlagDAGasMultiplier, def.Rollkit.DAGasMultiplier, "DA gas price multiplier for retrying blob transactions")
	cmd.Flags().Uint64(FlagDAStartHeight, def.Rollkit.DAStartHeight, "starting DA block height (for syncing)")
	cmd.Flags().String(FlagDANamespace, def.Rollkit.DANamespace, "DA namespace to submit blob transactions")
	cmd.Flags().String(FlagDASubmitOptions, def.Rollkit.DASubmitOptions, "DA submit options")
	cmd.Flags().Bool(FlagLight, def.Rollkit.Light, "run light client")
	cmd.Flags().String(FlagTrustedHash, def.Rollkit.TrustedHash, "initial trusted hash to start the header exchange service")
	cmd.Flags().Uint64(FlagMaxPendingBlocks, def.Rollkit.MaxPendingBlocks, "limit of blocks pending DA submission (0 for no limit)")
	cmd.Flags().Uint64(FlagDAMempoolTTL, def.Rollkit.DAMempoolTTL, "number of DA blocks until transaction is dropped from the mempool")
	cmd.Flags().Duration(FlagLazyBlockTime, def.Rollkit.LazyBlockTime, "block time (for lazy mode)")
	cmd.Flags().String(FlagSequencerAddress, def.Rollkit.SequencerAddress, "sequencer middleware address (host:port)")
	cmd.Flags().String(FlagSequencerRollupID, def.Rollkit.SequencerRollupID, "sequencer middleware rollup ID (default: mock-rollup)")
	cmd.Flags().String(FlagExecutorAddress, def.Rollkit.ExecutorAddress, "executor middleware address (host:port)")

	// Add new flags for DBPath and Instrumentation
	cmd.Flags().String(FlagDBPath, def.Rollkit.DBPath, "database path relative to root directory")

	// Add instrumentation flags with default values from DefaultInstrumentationConfig
	instrDef := DefaultInstrumentationConfig()
	cmd.Flags().Bool(FlagPrometheus, instrDef.Prometheus, "enable Prometheus metrics")
	cmd.Flags().String(FlagPrometheusListenAddr, instrDef.PrometheusListenAddr, "Prometheus metrics listen address")
	cmd.Flags().Int(FlagMaxOpenConnections, instrDef.MaxOpenConnections, "maximum number of simultaneous connections for metrics")
}
