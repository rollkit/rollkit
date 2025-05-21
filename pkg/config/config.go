package config

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"time"

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
	FlagLazyAggregator = "rollkit.node.lazy_mode"
	// FlagMaxPendingHeaders is a flag to limit and pause block production when too many headers are waiting for DA confirmation
	FlagMaxPendingHeaders = "rollkit.node.max_pending_headers"
	// FlagLazyBlockTime is a flag for specifying the maximum interval between blocks in lazy aggregation mode
	FlagLazyBlockTime = "rollkit.node.lazy_block_interval"

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
	// FlagP2PPeers is a flag for specifying the P2P peers
	FlagP2PPeers = "rollkit.p2p.peers"
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
)

// Config stores Rollkit configuration.
type Config struct {
	// Base configuration
	RootDir string `mapstructure:"-" yaml:"-" comment:"Root directory where rollkit files are located"`
	DBPath  string `mapstructure:"db_path" yaml:"db_path" comment:"Path inside the root directory where the database is located"`
	ChainID string `mapstructure:"chain_id" yaml:"chain_id" comment:"Chain ID for the rollup"`
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
	BlockTime         DurationWrapper `mapstructure:"block_time" yaml:"block_time" comment:"Block time (duration). Examples: \"500ms\", \"1s\", \"5s\", \"1m\", \"2m30s\", \"10m\"."`
	MaxPendingHeaders uint64          `mapstructure:"max_pending_headers" yaml:"max_pending_headers" comment:"Maximum number of headers pending DA submission. When this limit is reached, the aggregator pauses block production until some headers are confirmed. Use 0 for no limit."`
	LazyMode          bool            `mapstructure:"lazy_mode" yaml:"lazy_mode" comment:"Enables lazy aggregation mode, where blocks are only produced when transactions are available or after LazyBlockTime. Optimizes resources by avoiding empty block creation during periods of inactivity."`
	LazyBlockInterval DurationWrapper `mapstructure:"lazy_block_interval" yaml:"lazy_block_interval" comment:"Maximum interval between blocks in lazy aggregation mode (LazyAggregator). Ensures blocks are produced periodically even without transactions to keep the chain active. Generally larger than BlockTime."`

	// Header configuration
	TrustedHash string `mapstructure:"trusted_hash" yaml:"trusted_hash" comment:"Initial trusted hash used to bootstrap the header exchange service. Allows nodes to start synchronizing from a specific trusted point in the chain instead of genesis. When provided, the node will fetch the corresponding header/block from peers using this hash and use it as a starting point for synchronization. If not provided, the node will attempt to fetch the genesis block instead."`

	// Pruning management configuration
	Pruning PruningConfig `mapstructure:"pruning" yaml:"pruning"`
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
	Peers         string `mapstructure:"peers" yaml:"peers" comment:"Comma separated list of peers to connect to"`
	BlockedPeers  string `mapstructure:"blocked_peers" yaml:"blocked_peers" comment:"Comma separated list of peer IDs to block from connecting"`
	AllowedPeers  string `mapstructure:"allowed_peers" yaml:"allowed_peers" comment:"Comma separated list of peer IDs to allow connections from"`
}

// SignerConfig contains all signer configuration parameters
type SignerConfig struct {
	SignerType string `mapstructure:"signer_type" yaml:"signer_type" comment:"Type of remote signer to use (file, grpc)"`
	SignerPath string `mapstructure:"signer_path" yaml:"signer_path" comment:"Path to the signer file or address"`
}

// RPCConfig contains all RPC server configuration parameters
type RPCConfig struct {
	Address string `mapstructure:"address" yaml:"address" comment:"Address to bind the RPC server to (host:port). Default: 127.0.0.1:7331"`
}

// Validate ensures that the root directory exists.
// It creates the directory if it does not exist.
func (c *Config) Validate() error {
	if c.RootDir == "" {
		return fmt.Errorf("root directory cannot be empty")
	}

	fullDir := filepath.Dir(c.ConfigPath())
	if err := os.MkdirAll(fullDir, 0o750); err != nil {
		return fmt.Errorf("could not create directory %q: %w", fullDir, err)
	}

	return nil
}

// ConfigPath returns the path to the configuration file.
func (c *Config) ConfigPath() string {
	return filepath.Join(c.RootDir, AppConfigDir, ConfigName)
}

// AddGlobalFlags registers the basic configuration flags that are common across applications.
// This includes logging configuration and root directory settings.
// It should be used in apps that do not already define their logger and home flag.
func AddGlobalFlags(cmd *cobra.Command, defaultHome string) {
	cmd.PersistentFlags().String(FlagLogLevel, DefaultConfig.Log.Level, "Set the log level (debug, info, warn, error)")
	cmd.PersistentFlags().String(FlagLogFormat, DefaultConfig.Log.Format, "Set the log format (text, json)")
	cmd.PersistentFlags().Bool(FlagLogTrace, DefaultConfig.Log.Trace, "Enable stack traces in error logs")
	cmd.PersistentFlags().String(FlagRootDir, DefaultRootDirWithName(defaultHome), "Root directory for application data")
}

// AddFlags adds Rollkit specific configuration options to cobra Command.
func AddFlags(cmd *cobra.Command) {
	def := DefaultConfig

	// Add base flags
	cmd.Flags().String(FlagDBPath, def.DBPath, "path for the node database")
	cmd.Flags().String(FlagChainID, def.ChainID, "chain ID")

	// Node configuration flags
	cmd.Flags().Bool(FlagAggregator, def.Node.Aggregator, "run node in aggregator mode")
	cmd.Flags().Bool(FlagLight, def.Node.Light, "run light client")
	cmd.Flags().Duration(FlagBlockTime, def.Node.BlockTime.Duration, "block time (for aggregator mode)")
	cmd.Flags().String(FlagTrustedHash, def.Node.TrustedHash, "initial trusted hash to start the header exchange service")
	cmd.Flags().Bool(FlagLazyAggregator, def.Node.LazyMode, "produce blocks only when transactions are available or after lazy block time")
	cmd.Flags().Uint64(FlagMaxPendingHeaders, def.Node.MaxPendingHeaders, "maximum headers pending DA confirmation before pausing block production (0 for no limit)")
	cmd.Flags().Duration(FlagLazyBlockTime, def.Node.LazyBlockInterval.Duration, "maximum interval between blocks in lazy aggregation mode")

	// Pruning configuration flags
	cmd.Flags().String(FlagPruningStrategy, def.Node.Pruning.Strategy, "pruning strategy (none, default, everything, custom)")
	cmd.Flags().Uint64(FlagPruningKeepRecent, def.Node.Pruning.KeepRecent, "number of recent blocks to keep")
	cmd.Flags().Uint64(FlagPruningInterval, def.Node.Pruning.Interval, "frequency of pruning operations")

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
	cmd.Flags().String(FlagP2PPeers, def.P2P.Peers, "Comma separated list of seed nodes to connect to")
	cmd.Flags().String(FlagP2PBlockedPeers, def.P2P.BlockedPeers, "Comma separated list of nodes to ignore")
	cmd.Flags().String(FlagP2PAllowedPeers, def.P2P.AllowedPeers, "Comma separated list of nodes to whitelist")

	// RPC configuration flags
	cmd.Flags().String(FlagRPCAddress, def.RPC.Address, "RPC server address (host:port)")

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

// Load loads the node configuration in the following order of precedence:
// 1. DefaultNodeConfig() (lowest priority)
// 2. YAML configuration file
// 3. Command line flags (highest priority)
func Load(cmd *cobra.Command) (Config, error) {
	home, _ := cmd.Flags().GetString(FlagRootDir)
	if home == "" {
		home = DefaultRootDir
	}

	v := viper.New()
	v.SetConfigName(ConfigFileName)
	v.SetConfigType(ConfigExtension)
	v.AddConfigPath(filepath.Join(home, AppConfigDir))
	v.AddConfigPath(filepath.Join(home, AppConfigDir, ConfigName))
	v.SetConfigFile(filepath.Join(home, AppConfigDir, ConfigName))
	_ = v.BindPFlags(cmd.Flags())
	_ = v.BindPFlags(cmd.PersistentFlags())
	v.AutomaticEnv()

	// get the executable name
	executableName, err := os.Executable()
	if err != nil {
		return Config{}, err
	}

	if err := bindFlags(path.Base(executableName), cmd, v); err != nil {
		return Config{}, err
	}

	// read the configuration file
	// if the configuration file does not exist, we ignore the error
	// it will use the defaults
	_ = v.ReadInConfig()

	return loadFromViper(v, home)
}

// LoadFromViper loads the node configuration from a provided viper instance.
// It gets the home directory from the input viper, sets up a new viper instance
// to read the config file, and then merges both instances.
// This allows getting configuration values from both command line flags (or other sources)
// and the config file, with the same precedence as Load.
func LoadFromViper(inputViper *viper.Viper) (Config, error) {
	// get home directory from input viper
	home := inputViper.GetString(FlagRootDir)
	if home == "" {
		home = DefaultRootDir
	}

	// create a new viper instance for reading the config file
	fileViper := viper.New()
	fileViper.SetConfigName(ConfigFileName)
	fileViper.SetConfigType(ConfigExtension)
	fileViper.AddConfigPath(filepath.Join(home, AppConfigDir))
	fileViper.AddConfigPath(filepath.Join(home, AppConfigDir, ConfigName))
	fileViper.SetConfigFile(filepath.Join(home, AppConfigDir, ConfigName))

	// read the configuration file
	// if the configuration file does not exist, we ignore the error
	// it will use the defaults
	_ = fileViper.ReadInConfig()

	// create a merged viper with input viper taking precedence
	mergedViper := viper.New()

	// first copy all settings from file viper
	for _, key := range fileViper.AllKeys() {
		mergedViper.Set(key, fileViper.Get(key))
	}

	// then override with settings from input viper (higher precedence)
	for _, key := range inputViper.AllKeys() {
		// Handle special case for prefixed keys
		if after, ok := strings.CutPrefix(key, "rollkit."); ok {
			// Strip the prefix for the merged viper
			strippedKey := after
			mergedViper.Set(strippedKey, inputViper.Get(key))
		} else {
			mergedViper.Set(key, inputViper.Get(key))
		}
	}

	return loadFromViper(mergedViper, home)
}

// loadFromViper is a helper function that processes a viper instance and returns a Config.
// It is used by both Load and LoadFromViper to ensure consistent behavior.
func loadFromViper(v *viper.Viper, home string) (Config, error) {
	cfg := DefaultConfig
	cfg.RootDir = home

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
			func(f reflect.Type, t reflect.Type, data any) (any, error) {
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
		),
		Result:           &cfg,
		WeaklyTypedInput: true,
	})
	if err != nil {
		return cfg, errors.Join(ErrReadYaml, fmt.Errorf("failed creating decoder: %w", err))
	}

	if err := decoder.Decode(v.AllSettings()); err != nil {
		return cfg, errors.Join(ErrReadYaml, fmt.Errorf("failed decoding viper: %w", err))
	}

	return cfg, nil
}

func bindFlags(basename string, cmd *cobra.Command, v *viper.Viper) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("bindFlags failed: %v", r)
		}
	}()

	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		flagName := strings.TrimPrefix(f.Name, "rollkit.") // trimm the prefix from the flag name

		// Environment variables can't have dashes in them, so bind them to their equivalent
		// keys with underscores, e.g. --favorite-color to STING_FAVORITE_COLOR
		err = v.BindEnv(flagName, fmt.Sprintf("%s_%s", basename, strings.ToUpper(strings.ReplaceAll(flagName, "-", "_"))))
		if err != nil {
			panic(err)
		}

		err = v.BindPFlag(flagName, f)
		if err != nil {
			panic(err)
		}

		// Apply the viper config value to the flag when the flag is not set and
		// viper has a value.
		if !f.Changed && v.IsSet(flagName) {
			val := v.Get(flagName)
			err = cmd.Flags().Set(flagName, fmt.Sprintf("%v", val))
			if err != nil {
				panic(err)
			}
		}
	})

	return err
}
