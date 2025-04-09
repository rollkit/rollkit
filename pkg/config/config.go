package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	// Base configuration flags

	// FlagRootDir is a flag for specifying the root directory
	FlagRootDir = "home"
	// FlagDBPath is a flag for specifying the database path
	FlagDBPath = "rollkit.db_path"
	// FlagChainConfigDir is a flag for specifying the chain config directory
	FlagChainConfigDir = "config_dir"
	// FlagChainID is a flag for specifying the chain ID
	FlagChainID = "chain_id"

	// Node configuration flags

	// FlagAggregator is a flag for running node in aggregator mode
	FlagAggregator = "rollkit.node.aggregator"
	// FlagLight is a flag for running the node in light mode
	FlagLight = "rollkit.node.light"
	// FlagBlockTime is a flag for specifying the block time
	FlagBlockTime = "rollkit.node.block_time"
	// FlagTrustedHash is a flag for specifying the trusted hash
	FlagTrustedHash = "rollkit.node.trusted_hash"
	// FlagLazyAggregator is a flag for enabling lazy aggregation mode that only produces blocks when transactions are available
	FlagLazyAggregator = "rollkit.node.lazy_aggregator"
	// FlagMaxPendingBlocks is a flag to limit and pause block production when too many blocks are waiting for DA confirmation
	FlagMaxPendingBlocks = "rollkit.node.max_pending_blocks"
	// FlagLazyBlockTime is a flag for specifying the maximum interval between blocks in lazy aggregation mode
	FlagLazyBlockTime = "rollkit.node.lazy_block_time"

	// Data Availability configuration flags

	// FlagDAAddress is a flag for specifying the data availability layer address
	FlagDAAddress = "rollkit.da.address"
	// FlagDAAuthToken is a flag for specifying the data availability layer auth token
	FlagDAAuthToken = "rollkit.da.auth_token" // #nosec G101
	// FlagDABlockTime is a flag for specifying the data availability layer block time
	FlagDABlockTime = "rollkit.da.block_time"
	// FlagDAGasPrice is a flag for specifying the data availability layer gas price
	FlagDAGasPrice = "rollkit.da.gas_price"
	// FlagDAGasMultiplier is a flag for specifying the data availability layer gas price retry multiplier
	FlagDAGasMultiplier = "rollkit.da.gas_multiplier"
	// FlagDAStartHeight is a flag for specifying the data availability layer start height
	FlagDAStartHeight = "rollkit.da.start_height"
	// FlagDANamespace is a flag for specifying the DA namespace ID
	FlagDANamespace = "rollkit.da.namespace"
	// FlagDASubmitOptions is a flag for data availability submit options
	FlagDASubmitOptions = "rollkit.da.submit_options"
	// FlagDAMempoolTTL is a flag for specifying the DA mempool TTL
	FlagDAMempoolTTL = "rollkit.da.mempool_ttl"

	// P2P configuration flags

	// FlagP2PListenAddress is a flag for specifying the P2P listen address
	FlagP2PListenAddress = "rollkit.p2p.listen_address"
	// FlagP2PSeeds is a flag for specifying the P2P seeds
	FlagP2PSeeds = "rollkit.p2p.seeds"
	// FlagP2PBlockedPeers is a flag for specifying the P2P blocked peers
	FlagP2PBlockedPeers = "rollkit.p2p.blocked_peers"
	// FlagP2PAllowedPeers is a flag for specifying the P2P allowed peers
	FlagP2PAllowedPeers = "rollkit.p2p.allowed_peers"

	// Instrumentation configuration flags

	// FlagPrometheus is a flag for enabling Prometheus metrics
	FlagPrometheus = "rollkit.instrumentation.prometheus"
	// FlagPrometheusListenAddr is a flag for specifying the Prometheus listen address
	FlagPrometheusListenAddr = "rollkit.instrumentation.prometheus_listen_addr"
	// FlagMaxOpenConnections is a flag for specifying the maximum number of open connections
	FlagMaxOpenConnections = "rollkit.instrumentation.max_open_connections"
	// FlagPprof is a flag for enabling pprof profiling endpoints for runtime debugging
	FlagPprof = "rollkit.instrumentation.pprof"
	// FlagPprofListenAddr is a flag for specifying the pprof listen address
	FlagPprofListenAddr = "rollkit.instrumentation.pprof_listen_addr"

	// Logging configuration flags

	// FlagLogLevel is a flag for specifying the log level
	FlagLogLevel = "rollkit.log.level"
	// FlagLogFormat is a flag for specifying the log format
	FlagLogFormat = "rollkit.log.format"
	// FlagLogTrace is a flag for enabling stack traces in error logs
	FlagLogTrace = "rollkit.log.trace"

	// Signer configuration flags

	// FlagSignerType is a flag for specifying the signer type
	FlagSignerType = "rollkit.signer.type"
	// FlagSignerPath is a flag for specifying the signer path
	FlagSignerPath = "rollkit.signer.path"

	// FlagSignerPassphrase is a flag for specifying the signer passphrase
	//nolint:gosec
	FlagSignerPassphrase = "rollkit.signer.passphrase"

	// RPC configuration flags

	// FlagRPCAddress is a flag for specifying the RPC server address
	FlagRPCAddress = "rollkit.rpc.address"
	// FlagRPCPort is a flag for specifying the RPC server port
	FlagRPCPort = "rollkit.rpc.port"
)

// DurationWrapper is a wrapper for time.Duration that implements encoding.TextMarshaler and encoding.TextUnmarshaler
// needed for YAML marshalling/unmarshalling especially for time.Duration
type DurationWrapper struct {
	time.Duration
}

// MarshalText implements encoding.TextMarshaler to format the duration as text
func (d DurationWrapper) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler to parse the duration from text
func (d *DurationWrapper) UnmarshalText(text []byte) error {
	var err error
	d.Duration, err = time.ParseDuration(string(text))
	return err
}

// Config stores Rollkit configuration.
type Config struct {
	// Base configuration
	RootDir   string `mapstructure:"-" yaml:"-" comment:"Root directory where rollkit files are located"`
	DBPath    string `mapstructure:"db_path" yaml:"db_path" comment:"Path inside the root directory where the database is located"`
	ConfigDir string `mapstructure:"config_dir" yaml:"config_dir" comment:"Directory containing the rollup chain configuration"`
	ChainID   string `mapstructure:"chain_id" yaml:"chain_id" comment:"Chain ID for the rollup"`
	// P2P configuration
	P2P P2PConfig `mapstructure:"p2p" yaml:"p2p"`

	// Node specific configuration
	Node NodeConfig `mapstructure:"node" yaml:"node"`

	// Data availability configuration
	DA DAConfig `mapstructure:"da" yaml:"da"`

	// RPC configuration
	RPC RPCConfig `mapstructure:"rpc" yaml:"rpc"`

	// Instrumentation configuration
	Instrumentation *InstrumentationConfig `mapstructure:"instrumentation" yaml:"instrumentation"`

	// Logging configuration
	Log LogConfig `mapstructure:"log" yaml:"log"`

	// Remote signer configuration
	Signer SignerConfig `mapstructure:"signer" yaml:"signer"`
}

// DAConfig contains all Data Availability configuration parameters
type DAConfig struct {
	Address       string          `mapstructure:"address" yaml:"address" comment:"Address of the data availability layer service (host:port). This is the endpoint where Rollkit will connect to submit and retrieve data."`
	AuthToken     string          `mapstructure:"auth_token" yaml:"auth_token" comment:"Authentication token for the data availability layer service. Required if the DA service needs authentication."`
	GasPrice      float64         `mapstructure:"gas_price" yaml:"gas_price" comment:"Gas price for data availability transactions. Use -1 for automatic gas price determination. Higher values may result in faster inclusion."`
	GasMultiplier float64         `mapstructure:"gas_multiplier" yaml:"gas_multiplier" comment:"Multiplier applied to gas price when retrying failed DA submissions. Values > 1 increase gas price on retries to improve chances of inclusion."`
	SubmitOptions string          `mapstructure:"submit_options" yaml:"submit_options" comment:"Additional options passed to the DA layer when submitting data. Format depends on the specific DA implementation being used."`
	Namespace     string          `mapstructure:"namespace" yaml:"namespace" comment:"Namespace ID used when submitting blobs to the DA layer."`
	BlockTime     DurationWrapper `mapstructure:"block_time" yaml:"block_time" comment:"Average block time of the DA chain (duration). Determines frequency of DA layer syncing, maximum backoff time for retries, and is multiplied by MempoolTTL to calculate transaction expiration. Examples: \"15s\", \"30s\", \"1m\", \"2m30s\", \"10m\"."`
	StartHeight   uint64          `mapstructure:"start_height" yaml:"start_height" comment:"Starting block height on the DA layer from which to begin syncing. Useful when deploying a new rollup on an existing DA chain."`
	MempoolTTL    uint64          `mapstructure:"mempool_ttl" yaml:"mempool_ttl" comment:"Number of DA blocks after which a transaction is considered expired and dropped from the mempool. Controls retry backoff timing."`
}

// NodeConfig contains all Rollkit specific configuration parameters
type NodeConfig struct {
	// Node mode configuration
	Aggregator bool `yaml:"aggregator" comment:"Run node in aggregator mode"`
	Light      bool `yaml:"light" comment:"Run node in light mode"`

	// Block management configuration
	BlockTime        DurationWrapper `mapstructure:"block_time" yaml:"block_time" comment:"Block time (duration). Examples: \"500ms\", \"1s\", \"5s\", \"1m\", \"2m30s\", \"10m\"."`
	MaxPendingBlocks uint64          `mapstructure:"max_pending_blocks" yaml:"max_pending_blocks" comment:"Maximum number of blocks pending DA submission. When this limit is reached, the aggregator pauses block production until some blocks are confirmed. Use 0 for no limit."`
	LazyAggregator   bool            `mapstructure:"lazy_aggregator" yaml:"lazy_aggregator" comment:"Enables lazy aggregation mode, where blocks are only produced when transactions are available or after LazyBlockTime. Optimizes resources by avoiding empty block creation during periods of inactivity."`
	LazyBlockTime    DurationWrapper `mapstructure:"lazy_block_time" yaml:"lazy_block_time" comment:"Maximum interval between blocks in lazy aggregation mode (LazyAggregator). Ensures blocks are produced periodically even without transactions to keep the chain active. Generally larger than BlockTime."`

	// Header configuration
	TrustedHash string `mapstructure:"trusted_hash" yaml:"trusted_hash" comment:"Initial trusted hash used to bootstrap the header exchange service. Allows nodes to start synchronizing from a specific trusted point in the chain instead of genesis. When provided, the node will fetch the corresponding header/block from peers using this hash and use it as a starting point for synchronization. If not provided, the node will attempt to fetch the genesis block instead."`
}

// LogConfig contains all logging configuration parameters
type LogConfig struct {
	Level  string `mapstructure:"level" yaml:"level" comment:"Log level (debug, info, warn, error)"`
	Format string `mapstructure:"format" yaml:"format" comment:"Log format (text, json)"`
	Trace  bool   `mapstructure:"trace" yaml:"trace" comment:"Enable stack traces in error logs"`
}

// P2PConfig contains all peer-to-peer networking configuration parameters
type P2PConfig struct {
	ListenAddress string `mapstructure:"listen_address" yaml:"listen_address" comment:"Address to listen for incoming connections (host:port)"`
	Seeds         string `mapstructure:"seeds" yaml:"seeds" comment:"Comma separated list of seed nodes to connect to"`
	BlockedPeers  string `mapstructure:"blocked_peers" yaml:"blocked_peers" comment:"Comma separated list of peer IDs to block from connecting"`
	AllowedPeers  string `mapstructure:"allowed_peers" yaml:"allowed_peers" comment:"Comma separated list of peer IDs to allow connections from"`
}

// SignerConfig contains all signer configuration parameters
type SignerConfig struct {
	SignerType string `mapstructure:"signer_type" yaml:"signer_type" comment:"Type of remote signer to use (file, grpc)"`
	SignerPath string `mapstructure:"signer_path" yaml:"signer_path" comment:"Path to the signer file or address"`
}

// AddGlobalFlags registers the basic configuration flags that are common across applications
// This includes logging configuration and root directory settings
func AddGlobalFlags(cmd *cobra.Command, appName string) {
	cmd.PersistentFlags().String(FlagLogLevel, DefaultNodeConfig.Log.Level, "Set the log level (debug, info, warn, error)")
	cmd.PersistentFlags().String(FlagLogFormat, DefaultNodeConfig.Log.Format, "Set the log format (text, json)")
	cmd.PersistentFlags().Bool(FlagLogTrace, DefaultNodeConfig.Log.Trace, "Enable stack traces in error logs")
	cmd.PersistentFlags().String(FlagRootDir, DefaultRootDirWithName(appName), "Root directory for application data")
}

// RPCConfig contains all RPC server configuration parameters
type RPCConfig struct {
	Address string `mapstructure:"address" yaml:"address" comment:"Address to bind the RPC server to (host). Default: tcp://0.0.0.0"`
	Port    uint16 `mapstructure:"port" yaml:"port" comment:"Port to bind the RPC server to. Default: 26657"`
}

// AddFlags adds Rollkit specific configuration options to cobra Command.
func AddFlags(cmd *cobra.Command) {
	def := DefaultNodeConfig

	// Add CI flag for testing
	cmd.Flags().Bool("ci", false, "run node for ci testing")

	// Add base flags
	cmd.Flags().String(FlagDBPath, def.DBPath, "path for the node database")
	cmd.Flags().String(FlagChainConfigDir, def.ConfigDir, "directory containing chain configuration files")
	cmd.Flags().String(FlagChainID, def.ChainID, "chain ID")
	// Node configuration flags
	cmd.Flags().BoolVar(&def.Node.Aggregator, FlagAggregator, def.Node.Aggregator, "run node in aggregator mode")
	cmd.Flags().Bool(FlagLight, def.Node.Light, "run light client")
	cmd.Flags().Duration(FlagBlockTime, def.Node.BlockTime.Duration, "block time (for aggregator mode)")
	cmd.Flags().String(FlagTrustedHash, def.Node.TrustedHash, "initial trusted hash to start the header exchange service")
	cmd.Flags().Bool(FlagLazyAggregator, def.Node.LazyAggregator, "produce blocks only when transactions are available or after lazy block time")
	cmd.Flags().Uint64(FlagMaxPendingBlocks, def.Node.MaxPendingBlocks, "maximum blocks pending DA confirmation before pausing block production (0 for no limit)")
	cmd.Flags().Duration(FlagLazyBlockTime, def.Node.LazyBlockTime.Duration, "maximum interval between blocks in lazy aggregation mode")

	// Data Availability configuration flags
	cmd.Flags().String(FlagDAAddress, def.DA.Address, "DA address (host:port)")
	cmd.Flags().String(FlagDAAuthToken, def.DA.AuthToken, "DA auth token")
	cmd.Flags().Duration(FlagDABlockTime, def.DA.BlockTime.Duration, "DA chain block time (for syncing)")
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

	// RPC configuration flags
	cmd.Flags().String(FlagRPCAddress, def.RPC.Address, "RPC server address (host)")
	cmd.Flags().Uint16(FlagRPCPort, def.RPC.Port, "RPC server port")

	// Instrumentation configuration flags
	instrDef := DefaultInstrumentationConfig()
	cmd.Flags().Bool(FlagPrometheus, instrDef.Prometheus, "enable Prometheus metrics")
	cmd.Flags().String(FlagPrometheusListenAddr, instrDef.PrometheusListenAddr, "Prometheus metrics listen address")
	cmd.Flags().Int(FlagMaxOpenConnections, instrDef.MaxOpenConnections, "maximum number of simultaneous connections for metrics")
	cmd.Flags().Bool(FlagPprof, instrDef.Pprof, "enable pprof HTTP endpoint")
	cmd.Flags().String(FlagPprofListenAddr, instrDef.PprofListenAddr, "pprof HTTP server listening address")

	// Signer configuration flags
	cmd.Flags().String(FlagSignerType, def.Signer.SignerType, "type of signer to use (file, grpc)")
	cmd.Flags().String(FlagSignerPath, def.Signer.SignerPath, "path to the signer file or address")
	cmd.Flags().String(FlagSignerPassphrase, "", "passphrase for the signer (required for file signer and if aggregator is enabled)")
}

// LoadNodeConfig loads the node configuration in the following order of precedence:
// 1. DefaultNodeConfig() (lowest priority)
// 2. YAML configuration file
// 3. Command line flags (highest priority)
func LoadNodeConfig(cmd *cobra.Command, home string) (Config, error) {
	// Create a new Viper instance to avoid conflicts with any global Viper
	v := viper.New()

	// 1. Start with default configuration generated by the function
	//    and set defaults in Viper
	config := DefaultNodeConfig
	setDefaultsInViper(v, config)

	// Get the RootDir from flags *first* to ensure correct search paths
	// If the flag is not set, it will use the default value already in 'config'
	rootDirFromFlag, _ := cmd.Flags().GetString(FlagRootDir) // Ignore error, default is fine if flag not present
	if rootDirFromFlag != "" {
		config.RootDir = rootDirFromFlag // Update our config struct temporarily for path setting
	}

	// 2. Try to load YAML configuration from various locations
	// First try using the current directory
	v.SetConfigName(ConfigBaseName)  // e.g., "rollkit"
	v.SetConfigType(ConfigExtension) // e.g., "yaml"

	// Check if RootDir is determined (either default or from flag)
	if config.RootDir != "" {
		// Search directly in the determined root directory first
		v.AddConfigPath(home)
		// Then search in the default config subdirectory within that root directory
		v.AddConfigPath(filepath.Join(home, config.ConfigDir)) // DefaultConfigDir is likely "config"
	}

	// Try to read the config file
	if err := v.ReadInConfig(); err != nil {
		// If it's not a "file not found" error, return the error
		var configFileNotFound viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFound) {
			return config, fmt.Errorf("error reading YAML configuration: %w", err)
		}
		// Otherwise, just continue with defaults
	} else {
		// Config file found, log it
		fmt.Printf("Using config file: %s\n", v.ConfigFileUsed())
	}

	// 3. Bind command line flags
	var flagErrs error
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		// Always trim the rollkit prefix if it exists
		flagName := strings.TrimPrefix(f.Name, "rollkit.")
		if err := v.BindPFlag(flagName, f); err != nil {
			flagErrs = multierror.Append(flagErrs, err)
		}
	})
	if flagErrs != nil {
		return config, fmt.Errorf("unable to bind flags: %w", flagErrs)
	}

	// 4. Unmarshal everything from Viper into the config struct
	// viper.Unmarshal will respect the precedence: defaults < yaml < flags
	if err := v.Unmarshal(&config, func(c *mapstructure.DecoderConfig) {
		c.TagName = "mapstructure"
		c.DecodeHook = mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
			func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
				if t == reflect.TypeOf(DurationWrapper{}) && f.Kind() == reflect.String {
					if str, ok := data.(string); ok {
						duration, err := time.ParseDuration(str)
						if err != nil {
							return nil, err
						}
						return DurationWrapper{Duration: duration}, nil
					}
				}
				return data, nil
			},
		)
	}); err != nil {
		return config, fmt.Errorf("unable to decode configuration: %w", err)
	}

	return config, nil
}

// setDefaultsInViper sets all the default values from NodeConfig into Viper
func setDefaultsInViper(v *viper.Viper, config Config) {
	// This function needs to marshal the config to a map and set defaults
	// Example implementation sketch (might need refinement based on struct tags):
	configMap := make(map[string]interface{})
	data, _ := json.Marshal(config) // Using JSON temporarily, mapstructure might be better
	err := json.Unmarshal(data, &configMap)
	if err != nil {
		fmt.Println("error unmarshalling config", err)
	}

	for key, value := range configMap {
		v.SetDefault(key, value)
	}
	// Note: A proper implementation would recursively handle nested structs
	// and potentially use mapstructure tags used elsewhere in the loading process.
}
