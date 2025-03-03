package config

import (
	"time"

	cmcfg "github.com/cometbft/cometbft/config"
)

// NodeConfig stores Rollkit node configuration.
type NodeConfig struct {
	BaseConfig

	HeaderConfig       `mapstructure:",squash"`
	P2P                P2PConfig
	BlockManagerConfig `mapstructure:",squash"`
	Instrumentation    InstrumentationConfig  `mapstructure:"instrumentation"`
	DataAvailability   DataAvailabilityConfig `mapstructure:"data_availability"`
	Sequencer          SequencerConfig        `mapstructure:"sequencer"`

	// Light is a flag to run the node in light mode
	Light bool `mapstructure:"light"`

	// Aggregator signifies that the node is the block producer
	Aggregator bool `mapstructure:"aggregator"`

	ExecutorAddress string `mapstructure:"executor_address"`
}

// BaseConfig defines the base configuration for a CometBFT node
type BaseConfig struct { //nolint: maligned

	// The version of the CometBFT binary that created
	// or last modified the config file
	Version string `mapstructure:"version"`

	// The root directory for all data.
	// This should be set in viper so it can unmarshal into this struct
	RootDir string `mapstructure:"home"`

	// TCP or UNIX socket address of the ABCI application,
	// or the name of an ABCI application compiled in with the CometBFT binary
	ProxyApp string `mapstructure:"proxy_app"`

	// A custom human readable name for this node
	Moniker string `mapstructure:"moniker"`

	// Database directory
	DBPath string `mapstructure:"db_dir"`

	// Output level for logging
	LogLevel string `mapstructure:"log_level"`

	// Output format: 'plain' (colored text) or 'json'
	LogFormat string `mapstructure:"log_format"`

	// Path to the JSON file containing the initial validator set and other meta data
	Genesis string `mapstructure:"genesis_file"`

	// Path to the JSON file containing the private key to use as a validator in the consensus protocol
	PrivValidatorKey string `mapstructure:"priv_validator_key_file"`

	// Path to the JSON file containing the last sign state of a validator
	PrivValidatorState string `mapstructure:"priv_validator_state_file"`

	// TCP or UNIX socket address for CometBFT to listen on for
	// connections from an external PrivValidator process
	PrivValidatorListenAddr string `mapstructure:"priv_validator_laddr"`

	// A JSON file containing the private key to use for p2p authenticated encryption
	NodeKey string `mapstructure:"node_key_file"`

	// Mechanism to connect to the ABCI application: socket | grpc
	ABCI string `mapstructure:"abci"`

	// If true, query the ABCI app on connecting to a new peer
	// so the app can decide if we should keep the connection or not
	FilterPeers bool `mapstructure:"filter_peers"` // false
}

// HeaderConfig allows node to pass the initial trusted header hash to start the header exchange service
type HeaderConfig struct {
	TrustedHash string `mapstructure:"trusted_hash"`
}

// DataAvailabilityConfig consists of all parameters required by DataAvailabilityConfig
type DataAvailabilityConfig struct {
	// Namespace is the namespace for the data availability layer
	Namespace string `mapstructure:"namespace"`
	// AuthToken is the auth token for the data availability layer
	AuthToken string `mapstructure:"auth_token"`

	// GasPrice is the gas price for the data availability layer
	GasPrice float64 `mapstructure:"gas_price"`
	// GasMultiplier is the gas multiplier for the data availability layer
	GasMultiplier float64 `mapstructure:"gas_multiplier"`
	// SubmitOptions are the options for the data availability layer
	SubmitOptions string `mapstructure:"submit_options"`
}

type SequencerConfig struct {
	RollupID string `mapstructure:"rollup_id"`
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

// GetNodeConfig translates Tendermint's configuration into Rollkit configuration.
//
// This method only translates configuration, and doesn't verify it. If some option is missing in Tendermint's
// config, it's skipped during translation.
func GetNodeConfig(nodeConf *NodeConfig, cmConf *cmcfg.Config) {
	if cmConf != nil {
		nodeConf.RootDir = cmConf.RootDir
		nodeConf.DBPath = cmConf.DBPath
		if cmConf.P2P != nil {
			nodeConf.P2P.ListenAddress = cmConf.P2P.ListenAddress
			nodeConf.P2P.Seeds = cmConf.P2P.Seeds
		}
		if cmConf.Instrumentation != nil {
			nodeConf.Instrumentation = *DefaultInstrumentationConfig()
		}
	}
}
