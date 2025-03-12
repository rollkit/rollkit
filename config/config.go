package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	// Base configuration flags

	// FlagRootDir is a flag for specifying the root directory
	FlagRootDir = "home"
	// FlagDBPath is a flag for specifying the database path
	FlagDBPath = "db_path"
	// FlagEntrypoint is a flag for specifying the entrypoint
	FlagEntrypoint = "entrypoint"
	// FlagChainConfigDir is a flag for specifying the chain config directory
	FlagChainConfigDir = "chain.config_dir"

	// Node configuration flags

	// FlagAggregator is a flag for running node in aggregator mode
	FlagAggregator = "node.aggregator"
	// FlagLight is a flag for running the node in light mode
	FlagLight = "node.light"
	// FlagBlockTime is a flag for specifying the block time
	FlagBlockTime = "node.block_time"
	// FlagTrustedHash is a flag for specifying the trusted hash
	FlagTrustedHash = "node.trusted_hash"
	// FlagLazyAggregator is a flag for enabling lazy aggregation
	FlagLazyAggregator = "node.lazy_aggregator"
	// FlagMaxPendingBlocks is a flag to pause aggregator in case of large number of blocks pending DA submission
	FlagMaxPendingBlocks = "node.max_pending_blocks"
	// FlagLazyBlockTime is a flag for specifying the block time in lazy mode
	FlagLazyBlockTime = "node.lazy_block_time"
	// FlagSequencerAddress is a flag for specifying the sequencer middleware address
	FlagSequencerAddress = "node.sequencer_address"
	// FlagSequencerRollupID is a flag for specifying the sequencer middleware rollup ID
	FlagSequencerRollupID = "node.sequencer_rollup_id"
	// FlagExecutorAddress is a flag for specifying the sequencer middleware address
	FlagExecutorAddress = "node.executor_address"

	// Data Availability configuration flags

	// FlagDAAddress is a flag for specifying the data availability layer address
	FlagDAAddress = "da.address"
	// FlagDAAuthToken is a flag for specifying the data availability layer auth token
	FlagDAAuthToken = "da.auth_token" // #nosec G101
	// FlagDABlockTime is a flag for specifying the data availability layer block time
	FlagDABlockTime = "da.block_time"
	// FlagDAGasPrice is a flag for specifying the data availability layer gas price
	FlagDAGasPrice = "da.gas_price"
	// FlagDAGasMultiplier is a flag for specifying the data availability layer gas price retry multiplier
	FlagDAGasMultiplier = "da.gas_multiplier"
	// FlagDAStartHeight is a flag for specifying the data availability layer start height
	FlagDAStartHeight = "da.start_height"
	// FlagDANamespace is a flag for specifying the DA namespace ID
	FlagDANamespace = "da.namespace"
	// FlagDASubmitOptions is a flag for data availability submit options
	FlagDASubmitOptions = "da.submit_options"
	// FlagDAMempoolTTL is a flag for specifying the DA mempool TTL
	FlagDAMempoolTTL = "da.mempool_ttl"

	// P2P configuration flags

	// FlagP2PListenAddress is a flag for specifying the P2P listen address
	FlagP2PListenAddress = "p2p.listen_address"
	// FlagP2PSeeds is a flag for specifying the P2P seeds
	FlagP2PSeeds = "p2p.seeds"
	// FlagP2PBlockedPeers is a flag for specifying the P2P blocked peers
	FlagP2PBlockedPeers = "p2p.blocked_peers"
	// FlagP2PAllowedPeers is a flag for specifying the P2P allowed peers
	FlagP2PAllowedPeers = "p2p.allowed_peers"

	// Instrumentation configuration flags

	// FlagPrometheus is a flag for enabling Prometheus metrics
	FlagPrometheus = "instrumentation.prometheus"
	// FlagPrometheusListenAddr is a flag for specifying the Prometheus listen address
	FlagPrometheusListenAddr = "instrumentation.prometheus_listen_addr"
	// FlagMaxOpenConnections is a flag for specifying the maximum number of open connections
	FlagMaxOpenConnections = "instrumentation.max_open_connections"
	// FlagPprof is a flag for enabling pprof profiling endpoints for runtime debugging
	FlagPprof = "instrumentation.pprof"
	// FlagPprofListenAddr is a flag for specifying the pprof listen address
	FlagPprofListenAddr = "instrumentation.pprof_listen_addr"

	// Logging configuration flags

	// FlagLogLevel is a flag for specifying the log level
	FlagLogLevel = "log.level"
	// FlagLogFormat is a flag for specifying the log format
	FlagLogFormat = "log.format"
	// FlagLogTrace is a flag for enabling stack traces in error logs
	FlagLogTrace = "log.trace"
)

// Config stores Rollkit configuration.
type Config struct {
	// Base configuration
	RootDir    string      `mapstructure:"home"`
	DBPath     string      `mapstructure:"db_path"`
	Entrypoint string      `mapstructure:"entrypoint" toml:"entrypoint"`
	Chain      ChainConfig `mapstructure:"chain" toml:"chain"`

	// P2P configuration
	P2P P2PConfig `mapstructure:"p2p"`

	// Node specific configuration
	Node NodeConfig `mapstructure:"node"`

	// Data availability configuration
	DA DAConfig `mapstructure:"da"`

	// Instrumentation configuration
	Instrumentation *InstrumentationConfig `mapstructure:"instrumentation"`

	// Logging configuration
	Log LogConfig `mapstructure:"log"`
}

// DAConfig contains all Data Availability configuration parameters
type DAConfig struct {
	Address       string        `mapstructure:"address" toml:"address"`
	AuthToken     string        `mapstructure:"auth_token" toml:"auth_token"`
	GasPrice      float64       `mapstructure:"gas_price" toml:"gas_price"`
	GasMultiplier float64       `mapstructure:"gas_multiplier" toml:"gas_multiplier"`
	SubmitOptions string        `mapstructure:"submit_options" toml:"submit_options"`
	Namespace     string        `mapstructure:"namespace" toml:"namespace"`
	BlockTime     time.Duration `mapstructure:"block_time" toml:"block_time"`
	StartHeight   uint64        `mapstructure:"start_height" toml:"start_height"`
	MempoolTTL    uint64        `mapstructure:"mempool_ttl" toml:"mempool_ttl"`
}

// NodeConfig contains all Rollkit specific configuration parameters
type NodeConfig struct {
	// Node mode configuration
	Aggregator bool `toml:"aggregator"`
	Light      bool `toml:"light"`

	// Block management configuration
	BlockTime        time.Duration `mapstructure:"block_time" toml:"block_time"`
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

// LogConfig contains all logging configuration parameters
type LogConfig struct {
	// Level is the log level (debug, info, warn, error)
	Level string `mapstructure:"level" toml:"level"`
	// Format is the log format (text, json)
	Format string `mapstructure:"format" toml:"format"`
	// Trace enables stack traces in error logs
	Trace bool `mapstructure:"trace" toml:"trace"`
}

// AddFlags adds Rollkit specific configuration options to cobra Command.
//
// This function is called in cosmos-sdk.
func AddFlags(cmd *cobra.Command) {
	def := DefaultNodeConfig

	// Base configuration flags
	cmd.Flags().String(FlagRootDir, def.RootDir, "root directory for Rollkit")
	cmd.Flags().String(FlagDBPath, def.DBPath, "database path relative to root directory")
	cmd.Flags().String(FlagEntrypoint, def.Entrypoint, "entrypoint for the application")
	cmd.Flags().String(FlagChainConfigDir, def.Chain.ConfigDir, "chain configuration directory")

	// Node configuration flags
	cmd.Flags().BoolVar(&def.Node.Aggregator, FlagAggregator, def.Node.Aggregator, "run node in aggregator mode")
	cmd.Flags().Bool(FlagLight, def.Node.Light, "run light client")
	cmd.Flags().Duration(FlagBlockTime, def.Node.BlockTime, "block time (for aggregator mode)")
	cmd.Flags().String(FlagTrustedHash, def.Node.TrustedHash, "initial trusted hash to start the header exchange service")
	cmd.Flags().Bool(FlagLazyAggregator, def.Node.LazyAggregator, "wait for transactions, don't build empty blocks")
	cmd.Flags().Uint64(FlagMaxPendingBlocks, def.Node.MaxPendingBlocks, "limit of blocks pending DA submission (0 for no limit)")
	cmd.Flags().Duration(FlagLazyBlockTime, def.Node.LazyBlockTime, "block time (for lazy mode)")
	cmd.Flags().String(FlagSequencerAddress, def.Node.SequencerAddress, "sequencer middleware address (host:port)")
	cmd.Flags().String(FlagSequencerRollupID, def.Node.SequencerRollupID, "sequencer middleware rollup ID (default: mock-rollup)")
	cmd.Flags().String(FlagExecutorAddress, def.Node.ExecutorAddress, "executor middleware address (host:port)")

	// Data Availability configuration flags
	cmd.Flags().String(FlagDAAddress, def.DA.Address, "DA address (host:port)")
	cmd.Flags().String(FlagDAAuthToken, def.DA.AuthToken, "DA auth token")
	cmd.Flags().Duration(FlagDABlockTime, def.DA.BlockTime, "DA chain block time (for syncing)")
	cmd.Flags().Float64(FlagDAGasPrice, def.DA.GasPrice, "DA gas price for blob transactions")
	cmd.Flags().Float64(FlagDAGasMultiplier, def.DA.GasMultiplier, "DA gas price multiplier for retrying blob transactions")
	cmd.Flags().Uint64(FlagDAStartHeight, def.DA.StartHeight, "starting DA block height (for syncing)")
	cmd.Flags().String(FlagDANamespace, def.DA.Namespace, "DA namespace to submit blob transactions")
	cmd.Flags().String(FlagDASubmitOptions, def.DA.SubmitOptions, "DA submit options")
	cmd.Flags().Uint64(FlagDAMempoolTTL, def.DA.MempoolTTL, "number of DA blocks until transaction is dropped from the mempool")

	// P2P configuration flags
	cmd.Flags().String(FlagP2PListenAddress, def.P2P.ListenAddress, "P2P listen address (host:port)")
	cmd.Flags().String(FlagP2PSeeds, def.P2P.Seeds, "Comma separated list of seed nodes to connect to")
	cmd.Flags().String(FlagP2PBlockedPeers, def.P2P.BlockedPeers, "Comma separated list of nodes to ignore")
	cmd.Flags().String(FlagP2PAllowedPeers, def.P2P.AllowedPeers, "Comma separated list of nodes to whitelist")

	// Instrumentation configuration flags
	instrDef := DefaultInstrumentationConfig()
	cmd.Flags().Bool(FlagPrometheus, instrDef.Prometheus, "enable Prometheus metrics")
	cmd.Flags().String(FlagPrometheusListenAddr, instrDef.PrometheusListenAddr, "Prometheus metrics listen address")
	cmd.Flags().Int(FlagMaxOpenConnections, instrDef.MaxOpenConnections, "maximum number of simultaneous connections for metrics")
	cmd.Flags().Bool(FlagPprof, instrDef.Pprof, "enable pprof HTTP endpoint")
	cmd.Flags().String(FlagPprofListenAddr, instrDef.PprofListenAddr, "pprof HTTP server listening address")

	// Logging configuration flags
	cmd.Flags().String(FlagLogLevel, "info", "log level (debug, info, warn, error)")
	cmd.Flags().String(FlagLogFormat, "", "log format (text, json)")
	cmd.Flags().Bool(FlagLogTrace, false, "enable stack traces in error logs")
}

// LoadNodeConfig loads the node configuration in the following order of precedence:
// 1. DefaultNodeConfig (lowest priority)
// 2. TOML configuration file
// 3. Command line flags (highest priority)
func LoadNodeConfig(cmd *cobra.Command) (Config, error) {
	// Create a new Viper instance to avoid conflicts with any global Viper
	v := viper.New()

	// 1. Start with default configuration and set defaults in Viper
	config := DefaultNodeConfig
	setDefaultsInViper(v, config)

	// 2. Try to load TOML configuration from various locations
	// First try using the current directory
	v.SetConfigName(ConfigBaseName)
	v.SetConfigType(ConfigExtension)

	// Add search paths in order of precedence
	// Current directory
	v.AddConfigPath(".")

	// Check if RootDir is set in the default config
	if config.RootDir != "" {
		v.AddConfigPath(filepath.Join(config.RootDir, DefaultConfigDir))
	}

	// Try to read the config file
	if err := v.ReadInConfig(); err != nil {
		// If it's not a "file not found" error, return the error
		var configFileNotFound viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFound) {
			return config, fmt.Errorf("error reading TOML configuration: %w", err)
		}
		// Otherwise, just continue with defaults
	} else {
		// Config file found, log it
		fmt.Printf("Using config file: %s\n", v.ConfigFileUsed())
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
func setDefaultsInViper(v *viper.Viper, config Config) {
	// Base configuration defaults
	v.SetDefault(FlagRootDir, config.RootDir)
	v.SetDefault(FlagDBPath, config.DBPath)
	v.SetDefault(FlagEntrypoint, config.Entrypoint)
	v.SetDefault(FlagChainConfigDir, config.Chain.ConfigDir)

	// Node configuration defaults
	v.SetDefault(FlagAggregator, config.Node.Aggregator)
	v.SetDefault(FlagLight, config.Node.Light)
	v.SetDefault(FlagBlockTime, config.Node.BlockTime)
	v.SetDefault(FlagTrustedHash, config.Node.TrustedHash)
	v.SetDefault(FlagLazyAggregator, config.Node.LazyAggregator)
	v.SetDefault(FlagMaxPendingBlocks, config.Node.MaxPendingBlocks)
	v.SetDefault(FlagLazyBlockTime, config.Node.LazyBlockTime)
	v.SetDefault(FlagSequencerAddress, config.Node.SequencerAddress)
	v.SetDefault(FlagSequencerRollupID, config.Node.SequencerRollupID)
	v.SetDefault(FlagExecutorAddress, config.Node.ExecutorAddress)

	// Data Availability configuration defaults
	v.SetDefault(FlagDAAddress, config.DA.Address)
	v.SetDefault(FlagDAAuthToken, config.DA.AuthToken)
	v.SetDefault(FlagDABlockTime, config.DA.BlockTime)
	v.SetDefault(FlagDAGasPrice, config.DA.GasPrice)
	v.SetDefault(FlagDAGasMultiplier, config.DA.GasMultiplier)
	v.SetDefault(FlagDAStartHeight, config.DA.StartHeight)
	v.SetDefault(FlagDANamespace, config.DA.Namespace)
	v.SetDefault(FlagDASubmitOptions, config.DA.SubmitOptions)
	v.SetDefault(FlagDAMempoolTTL, config.DA.MempoolTTL)

	// P2P configuration defaults
	v.SetDefault(FlagP2PListenAddress, config.P2P.ListenAddress)
	v.SetDefault(FlagP2PSeeds, config.P2P.Seeds)
	v.SetDefault(FlagP2PBlockedPeers, config.P2P.BlockedPeers)
	v.SetDefault(FlagP2PAllowedPeers, config.P2P.AllowedPeers)

	// Instrumentation configuration defaults
	if config.Instrumentation != nil {
		v.SetDefault(FlagPrometheus, config.Instrumentation.Prometheus)
		v.SetDefault(FlagPrometheusListenAddr, config.Instrumentation.PrometheusListenAddr)
		v.SetDefault(FlagMaxOpenConnections, config.Instrumentation.MaxOpenConnections)
		v.SetDefault(FlagPprof, config.Instrumentation.Pprof)
		v.SetDefault(FlagPprofListenAddr, config.Instrumentation.PprofListenAddr)
	}

	// Logging configuration defaults
	v.SetDefault(FlagLogLevel, "info")
	v.SetDefault(FlagLogFormat, "")
	v.SetDefault(FlagLogTrace, false)
}
