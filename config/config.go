package config

import (
	"fmt"
	"os"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	// FlagRootDir is a flag for specifying the root directory
	FlagRootDir = "home"
	// FlagDBPath is a flag for specifying the database path
	FlagDBPath = "db_path"

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

	// FlagPrometheus is a flag for enabling Prometheus metrics
	FlagPrometheus = "instrumentation.prometheus"
	// FlagPrometheusListenAddr is a flag for specifying the Prometheus listen address
	FlagPrometheusListenAddr = "instrumentation.prometheus_listen_addr"
	// FlagMaxOpenConnections is a flag for specifying the maximum number of open connections
	FlagMaxOpenConnections = "instrumentation.max_open_connections"

	// FlagP2PListenAddress is a flag for specifying the P2P listen address
	FlagP2PListenAddress = "p2p.listen_address"
	// FlagP2PSeeds is a flag for specifying the P2P seeds
	FlagP2PSeeds = "p2p.seeds"
	// FlagP2PBlockedPeers is a flag for specifying the P2P blocked peers
	FlagP2PBlockedPeers = "p2p.blocked_peers"
	// FlagP2PAllowedPeers is a flag for specifying the P2P allowed peers
	FlagP2PAllowedPeers = "p2p.allowed_peers"

	// FlagEntrypoint is a flag for specifying the entrypoint
	FlagEntrypoint = "entrypoint"
	// FlagChainConfigDir is a flag for specifying the chain config directory
	FlagChainConfigDir = "chain.config_dir"
)

// NodeConfig stores Rollkit node configuration.
type NodeConfig struct {
	// parameters below are translated from existing config
	RootDir string `mapstructure:"home"`
	DBPath  string `mapstructure:"db_path"`

	// P2P configuration
	P2P P2PConfig `mapstructure:"p2p"`

	// Rollkit specific configuration
	Rollkit RollkitConfig `mapstructure:"rollkit"`

	// Instrumentation configuration
	Instrumentation *InstrumentationConfig `mapstructure:"instrumentation"`

	// TOML configuration
	Entrypoint string      `mapstructure:"entrypoint" toml:"entrypoint"`
	Chain      ChainConfig `mapstructure:"chain" toml:"chain"`
}

// RollkitConfig contains all Rollkit specific configuration parameters
type RollkitConfig struct {
	// Node mode configuration
	Aggregator bool `toml:"aggregator"`
	Light      bool `toml:"light"`

	// Data availability configuration
	DAAddress       string  `mapstructure:"da_address" toml:"da_address"`
	DAAuthToken     string  `mapstructure:"da_auth_token" toml:"da_auth_token"`
	DAGasPrice      float64 `mapstructure:"da_gas_price" toml:"da_gas_price"`
	DAGasMultiplier float64 `mapstructure:"da_gas_multiplier" toml:"da_gas_multiplier"`
	DASubmitOptions string  `mapstructure:"da_submit_options" toml:"da_submit_options"`
	DANamespace     string  `mapstructure:"da_namespace" toml:"da_namespace"`

	// Block management configuration
	BlockTime        time.Duration `mapstructure:"block_time" toml:"block_time"`
	DABlockTime      time.Duration `mapstructure:"da_block_time" toml:"da_block_time"`
	DAStartHeight    uint64        `mapstructure:"da_start_height" toml:"da_start_height"`
	DAMempoolTTL     uint64        `mapstructure:"da_mempool_ttl" toml:"da_mempool_ttl"`
	MaxPendingBlocks uint64        `mapstructure:"max_pending_blocks" toml:"max_pending_blocks"`
	LazyAggregator   bool          `mapstructure:"lazy_aggregator" toml:"lazy_aggregator"`
	LazyBlockTime    time.Duration `mapstructure:"lazy_block_time" toml:"lazy_block_time"`

	// Header configuration
	TrustedHash string `mapstructure:"trusted_hash" toml:"trusted_hash"`

	// Sequencer configuration
	SequencerAddress  string `mapstructure:"sequencer_address" toml:"sequencer_address"`
	SequencerRollupID string `mapstructure:"sequencer_rollup_id" toml:"sequencer_rollup_id"`
	ExecutorAddress   string `mapstructure:"executor_address" toml:"executor_address"`
}

// ChainConfig is the configuration for the chain section
type ChainConfig struct {
	ConfigDir string `mapstructure:"config_dir" toml:"config_dir"`
}

// AddFlags adds Rollkit specific configuration options to cobra Command.
//
// This function is called in cosmos-sdk.
func AddFlags(cmd *cobra.Command) {
	def := DefaultNodeConfig

	cmd.Flags().String(FlagRootDir, def.RootDir, "root directory for Rollkit")
	cmd.Flags().String(FlagDBPath, def.DBPath, "database path relative to root directory")

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

	// Add instrumentation flags with default values from DefaultInstrumentationConfig
	instrDef := DefaultInstrumentationConfig()
	cmd.Flags().Bool(FlagPrometheus, instrDef.Prometheus, "enable Prometheus metrics")
	cmd.Flags().String(FlagPrometheusListenAddr, instrDef.PrometheusListenAddr, "Prometheus metrics listen address")
	cmd.Flags().Int(FlagMaxOpenConnections, instrDef.MaxOpenConnections, "maximum number of simultaneous connections for metrics")

	// Add P2P flags
	cmd.Flags().String(FlagP2PListenAddress, def.P2P.ListenAddress, "P2P listen address (host:port)")
	cmd.Flags().String(FlagP2PSeeds, def.P2P.Seeds, "Comma separated list of seed nodes to connect to")
	cmd.Flags().String(FlagP2PBlockedPeers, def.P2P.BlockedPeers, "Comma separated list of nodes to ignore")
	cmd.Flags().String(FlagP2PAllowedPeers, def.P2P.AllowedPeers, "Comma separated list of nodes to whitelist")

	// Add TOML config flags
	cmd.Flags().String(FlagEntrypoint, def.Entrypoint, "entrypoint for the application")
	cmd.Flags().String(FlagChainConfigDir, def.Chain.ConfigDir, "chain configuration directory")
}

// LoadNodeConfig loads the node configuration in the following order of precedence:
// 1. DefaultNodeConfig (lowest priority)
// 2. TOML configuration file
// 3. Command line flags (highest priority)
func LoadNodeConfig(cmd *cobra.Command) (NodeConfig, error) {
	// Create a new Viper instance to avoid conflicts with any global Viper
	v := viper.New()

	// 1. Start with default configuration and set defaults in Viper
	config := DefaultNodeConfig
	setDefaultsInViper(v, config)

	// 2. Try to load TOML configuration from the current directory
	v.SetConfigName(RollkitToml[:len(RollkitToml)-5]) // Remove the .toml extension
	v.SetConfigType("toml")
	v.AddConfigPath(".") // Look in the current directory

	if err := v.ReadInConfig(); err == nil {
		fmt.Printf("Using config file: %s\n", v.ConfigFileUsed())
	} else if !os.IsNotExist(err) {
		// If it's not a "file not found" error, return the error
		return config, fmt.Errorf("error reading TOML configuration: %w", err)
	}

	// 3. Bind command line flags
	if err := v.BindPFlags(cmd.Flags()); err != nil {
		return config, fmt.Errorf("unable to bind flags: %w", err)
	}

	// 4. Unmarshal everything from Viper into the config struct
	if err := v.Unmarshal(&config, func(c *mapstructure.DecoderConfig) {
		c.TagName = "mapstructure"
		c.DecodeHook = mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		)
	}); err != nil {
		return config, fmt.Errorf("unable to decode configuration: %w", err)
	}

	return config, nil
}

// setDefaultsInViper sets all the default values from NodeConfig into Viper
func setDefaultsInViper(v *viper.Viper, config NodeConfig) {
	// Root level defaults
	v.SetDefault(FlagRootDir, config.RootDir)
	v.SetDefault(FlagDBPath, config.DBPath)
	v.SetDefault(FlagEntrypoint, config.Entrypoint)
	v.SetDefault(FlagChainConfigDir, config.Chain.ConfigDir)

	// P2P defaults
	v.SetDefault(FlagP2PListenAddress, config.P2P.ListenAddress)
	v.SetDefault(FlagP2PSeeds, config.P2P.Seeds)
	v.SetDefault(FlagP2PBlockedPeers, config.P2P.BlockedPeers)
	v.SetDefault(FlagP2PAllowedPeers, config.P2P.AllowedPeers)

	// Rollkit defaults
	v.SetDefault(FlagAggregator, config.Rollkit.Aggregator)
	v.SetDefault(FlagLight, config.Rollkit.Light)
	v.SetDefault(FlagDAAddress, config.Rollkit.DAAddress)
	v.SetDefault(FlagDAAuthToken, config.Rollkit.DAAuthToken)
	v.SetDefault(FlagBlockTime, config.Rollkit.BlockTime)
	v.SetDefault(FlagDABlockTime, config.Rollkit.DABlockTime)
	v.SetDefault(FlagDAGasPrice, config.Rollkit.DAGasPrice)
	v.SetDefault(FlagDAGasMultiplier, config.Rollkit.DAGasMultiplier)
	v.SetDefault(FlagDAStartHeight, config.Rollkit.DAStartHeight)
	v.SetDefault(FlagDANamespace, config.Rollkit.DANamespace)
	v.SetDefault(FlagDASubmitOptions, config.Rollkit.DASubmitOptions)
	v.SetDefault(FlagTrustedHash, config.Rollkit.TrustedHash)
	v.SetDefault(FlagLazyAggregator, config.Rollkit.LazyAggregator)
	v.SetDefault(FlagMaxPendingBlocks, config.Rollkit.MaxPendingBlocks)
	v.SetDefault(FlagDAMempoolTTL, config.Rollkit.DAMempoolTTL)
	v.SetDefault(FlagLazyBlockTime, config.Rollkit.LazyBlockTime)
	v.SetDefault(FlagSequencerAddress, config.Rollkit.SequencerAddress)
	v.SetDefault(FlagSequencerRollupID, config.Rollkit.SequencerRollupID)
	v.SetDefault(FlagExecutorAddress, config.Rollkit.ExecutorAddress)

	// Instrumentation defaults
	if config.Instrumentation != nil {
		v.SetDefault(FlagPrometheus, config.Instrumentation.Prometheus)
		v.SetDefault(FlagPrometheusListenAddr, config.Instrumentation.PrometheusListenAddr)
		v.SetDefault(FlagMaxOpenConnections, config.Instrumentation.MaxOpenConnections)
	}
}
