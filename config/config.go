package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
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
)

// NodeConfig stores Rollkit node configuration.
type NodeConfig struct {
	// parameters below are translated from existing config
	RootDir string
	DBPath  string
	P2P     P2PConfig
	// parameters below are Rollkit specific and read from config
	Aggregator         bool `mapstructure:"aggregator"`
	BlockManagerConfig `mapstructure:",squash"`
	DAAddress          string `mapstructure:"da_address"`
	DAAuthToken        string `mapstructure:"da_auth_token"`
	Light              bool   `mapstructure:"light"`
	HeaderConfig       `mapstructure:",squash"`
	Instrumentation    *InstrumentationConfig `mapstructure:"instrumentation"`
	DAGasPrice         float64                `mapstructure:"da_gas_price"`
	DAGasMultiplier    float64                `mapstructure:"da_gas_multiplier"`
	DASubmitOptions    string                 `mapstructure:"da_submit_options"`

	// CLI flags
	DANamespace       string `mapstructure:"da_namespace"`
	SequencerAddress  string `mapstructure:"sequencer_address"`
	SequencerRollupID string `mapstructure:"sequencer_rollup_id"`

	ExecutorAddress string `mapstructure:"executor_address"`
}

// HeaderConfig allows node to pass the initial trusted header hash to start the header exchange service
type HeaderConfig struct {
	TrustedHash string `mapstructure:"trusted_hash"`
}

// BlockManagerConfig consists of all parameters required by BlockManagerConfig
type BlockManagerConfig struct {
	// BlockTime defines how often new blocks are produced
	BlockTime time.Duration `mapstructure:"block_time"`
	// DABlockTime informs about block time of underlying data availability layer
	DABlockTime time.Duration `mapstructure:"da_block_time"`
	// DAStartHeight allows skipping first DAStartHeight-1 blocks when querying for blocks.
	DAStartHeight uint64 `mapstructure:"da_start_height"`
	// DAMempoolTTL is the number of DA blocks until transaction is dropped from the mempool.
	DAMempoolTTL uint64 `mapstructure:"da_mempool_ttl"`
	// MaxPendingBlocks defines limit of blocks pending DA submission. 0 means no limit.
	// When limit is reached, aggregator pauses block production.
	MaxPendingBlocks uint64 `mapstructure:"max_pending_blocks"`
	// LazyAggregator defines whether new blocks are produced in lazy mode
	LazyAggregator bool `mapstructure:"lazy_aggregator"`
	// LazyBlockTime defines how often new blocks are produced in lazy mode
	// even if there are no transactions
	LazyBlockTime time.Duration `mapstructure:"lazy_block_time"`
}

// InstrumentationConfig defines
type InstrumentationConfig struct {
	Prometheus           bool   `mapstructure:"prometheus"`
	PrometheusListenAddr string `mapstructure:"prometheus_listen_addr"`
	MaxOpenConnections   int    `mapstructure:"max_open_connections"`
	Namespace            string `mapstructure:"namespace"`
}

// GetNodeConfig translates Tendermint's configuration into Rollkit configuration.
//
// This method only translates configuration, and doesn't verify it. If some option is missing in Tendermint's
// config, it's skipped during translation.
func GetNodeConfig(nodeConf *NodeConfig) {
	if nodeConf.RootDir == "" {
		nodeConf.RootDir = DefaultNodeConfig.RootDir
	}

	if nodeConf.DBPath == "" {
		nodeConf.DBPath = filepath.Join(nodeConf.RootDir, "data")
	}
}

// GetViperConfig reads configuration parameters from Viper instance.
//
// This method is called in cosmos-sdk.
func (nc *NodeConfig) GetViperConfig(v *viper.Viper) error {
	nc.Aggregator = v.GetBool(FlagAggregator)
	nc.DAAddress = v.GetString(FlagDAAddress)
	nc.DAAuthToken = v.GetString(FlagDAAuthToken)
	nc.DAGasPrice = v.GetFloat64(FlagDAGasPrice)
	nc.DAGasMultiplier = v.GetFloat64(FlagDAGasMultiplier)
	nc.DANamespace = v.GetString(FlagDANamespace)
	nc.DAStartHeight = v.GetUint64(FlagDAStartHeight)
	nc.DABlockTime = v.GetDuration(FlagDABlockTime)
	nc.DASubmitOptions = v.GetString(FlagDASubmitOptions)
	nc.BlockTime = v.GetDuration(FlagBlockTime)
	nc.LazyAggregator = v.GetBool(FlagLazyAggregator)
	nc.Light = v.GetBool(FlagLight)
	nc.TrustedHash = v.GetString(FlagTrustedHash)
	nc.MaxPendingBlocks = v.GetUint64(FlagMaxPendingBlocks)
	nc.DAMempoolTTL = v.GetUint64(FlagDAMempoolTTL)
	nc.LazyBlockTime = v.GetDuration(FlagLazyBlockTime)
	nc.SequencerAddress = v.GetString(FlagSequencerAddress)
	nc.SequencerRollupID = v.GetString(FlagSequencerRollupID)
	nc.ExecutorAddress = v.GetString(FlagExecutorAddress)

	return nil
}

// AddFlags adds Rollkit specific configuration options to cobra Command.
//
// This function is called in cosmos-sdk.
func AddFlags(cmd *cobra.Command) {
	def := DefaultNodeConfig

	cmd.Flags().BoolVar(&def.Aggregator, FlagAggregator, def.Aggregator, "run node in aggregator mode")
	cmd.Flags().Bool(FlagLazyAggregator, def.LazyAggregator, "wait for transactions, don't build empty blocks")
	cmd.Flags().String(FlagDAAddress, def.DAAddress, "DA address (host:port)")
	cmd.Flags().String(FlagDAAuthToken, def.DAAuthToken, "DA auth token")
	cmd.Flags().Duration(FlagBlockTime, def.BlockTime, "block time (for aggregator mode)")
	cmd.Flags().Duration(FlagDABlockTime, def.DABlockTime, "DA chain block time (for syncing)")
	cmd.Flags().Float64(FlagDAGasPrice, def.DAGasPrice, "DA gas price for blob transactions")
	cmd.Flags().Float64(FlagDAGasMultiplier, def.DAGasMultiplier, "DA gas price multiplier for retrying blob transactions")
	cmd.Flags().Uint64(FlagDAStartHeight, def.DAStartHeight, "starting DA block height (for syncing)")
	cmd.Flags().String(FlagDANamespace, def.DANamespace, "DA namespace to submit blob transactions")
	cmd.Flags().String(FlagDASubmitOptions, def.DASubmitOptions, "DA submit options")
	cmd.Flags().Bool(FlagLight, def.Light, "run light client")
	cmd.Flags().String(FlagTrustedHash, def.TrustedHash, "initial trusted hash to start the header exchange service")
	cmd.Flags().Uint64(FlagMaxPendingBlocks, def.MaxPendingBlocks, "limit of blocks pending DA submission (0 for no limit)")
	cmd.Flags().Uint64(FlagDAMempoolTTL, def.DAMempoolTTL, "number of DA blocks until transaction is dropped from the mempool")
	cmd.Flags().Duration(FlagLazyBlockTime, def.LazyBlockTime, "block time (for lazy mode)")
	cmd.Flags().String(FlagSequencerAddress, def.SequencerAddress, "sequencer middleware address (host:port)")
	cmd.Flags().String(FlagSequencerRollupID, def.SequencerRollupID, "sequencer middleware rollup ID (default: mock-rollup)")
	cmd.Flags().String(FlagExecutorAddress, def.ExecutorAddress, "executor middleware address (host:port)")
}

// Config defines the configuration that replaces cometbft/config
type Config struct {
	// RootDir is the root directory for configuration files
	RootDir string `mapstructure:"home"`
	// ABCI specifies the ABCI connection type (socket | grpc)
	ABCI string `mapstructure:"abci"`
	// ProxyApp specifies the proxy application
	ProxyApp string `mapstructure:"proxy_app"`
	// P2P contains P2P configuration
	P2P P2PConfig `mapstructure:"p2p"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		RootDir:  defaultHomeDir(),
		ABCI:     "socket",
		ProxyApp: "noop",
		P2P: P2PConfig{
			ListenAddress: DefaultListenAddress,
		},
	}
}

// defaultHomeDir returns the default home directory
func defaultHomeDir() string {
	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE")
	}
	return filepath.Join(home, ".rollkit")
}

// ValidateBasic validates the basic configuration
func (cfg *Config) ValidateBasic() error {
	if cfg.RootDir == "" {
		return fmt.Errorf("root directory cannot be empty")
	}
	return nil
}

// GenesisFile returns the path to the genesis file
func (cfg *Config) GenesisFile() string {
	return filepath.Join(cfg.RootDir, "config", "genesis.json")
}

// NodeKeyFile returns the path to the node key file
func (cfg *Config) NodeKeyFile() string {
	return filepath.Join(cfg.RootDir, "config", "node_key.json")
}

// PrivValidatorKeyFile returns the path to the private validator key file
func (cfg *Config) PrivValidatorKeyFile() string {
	return filepath.Join(cfg.RootDir, "config", "priv_validator_key.json")
}

// PrivValidatorStateFile returns the path to the private validator state file
func (cfg *Config) PrivValidatorStateFile() string {
	return filepath.Join(cfg.RootDir, "data", "priv_validator_state.json")
}

// EnsureRoot ensures that the root directory exists
func EnsureRoot(rootDir string) {
	if err := os.MkdirAll(filepath.Join(rootDir, "config"), 0755); err != nil {
		panic(err)
	}
	if err := os.MkdirAll(filepath.Join(rootDir, "data"), 0755); err != nil {
		panic(err)
	}
}
