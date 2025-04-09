package config

import (
	"fmt"
	"os"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

// Added AggregatorConfig struct
type AggregatorConfig struct {
	QuorumThreshold int               `mapstructure:"quorum_threshold"` // Min signatures needed
	Attesters       map[string]string `mapstructure:"attesters"`        // AttesterID -> PublicKeyFilePath
}

// Added NetworkConfig struct
type NetworkConfig struct {
	// Addr for followers to send signatures to the leader.
	SequencerSigEndpoint string `mapstructure:"sequencer_sig_endpoint"`
	// Optional: Endpoint for block execution verification (if implemented)
	// FullnodeEndpoint   string `mapstructure:"fullnode_endpoint"`
}

// Added ExecutionConfig struct
type ExecutionConfig struct {
	Enabled          bool   `mapstructure:"enabled"`
	Type             string `mapstructure:"type"`              // e.g., "noop", "fullnode"
	FullnodeEndpoint string `mapstructure:"fullnode_endpoint"` // URL for the full node RPC/API
	Timeout          string `mapstructure:"timeout"`           // Duration string, e.g., "15s"

	// Parsed duration (not in YAML, calculated on load)
	parsedTimeout time.Duration
}

type Config struct {
	Node       NodeConfig       `mapstructure:"node"`
	Raft       RaftConfig       `mapstructure:"raft"`
	GRPC       GRPCConfig       `mapstructure:"grpc"`
	Signing    SigningConfig    `mapstructure:"signing"`
	Network    NetworkConfig    `mapstructure:"network"`
	Aggregator AggregatorConfig `mapstructure:"aggregator"`
	Execution  ExecutionConfig  `mapstructure:"execution"`
	// Possibly LoggingConfig, etc.
}

type NodeConfig struct {
	ID              string `mapstructure:"id"`                // Unique node ID within the RAFT cluster
	RaftBindAddress string `mapstructure:"raft_bind_address"` // Address:Port for internal RAFT communication
}

type RaftConfig struct {
	DataDir           string   `mapstructure:"data_dir"`           // Directory for logs, snapshots, stable store
	Peers             []string `mapstructure:"peers"`              // List of RAFT addresses of other nodes to join (optional if using discovery)
	BootstrapCluster  bool     `mapstructure:"bootstrap_cluster"`  // Should this node bootstrap a new cluster?
	ElectionTimeout   string   `mapstructure:"election_timeout"`   // Duration as string, e.g., "500ms"
	HeartbeatTimeout  string   `mapstructure:"heartbeat_timeout"`  // Duration as string, e.g., "50ms"
	SnapshotInterval  string   `mapstructure:"snapshot_interval"`  // Duration, e.g., "2m" (Snapshot interval)
	SnapshotThreshold uint64   `mapstructure:"snapshot_threshold"` // Number of commits to trigger snapshot

	// Parsed durations (not in TOML, calculated on load)
	parsedElectionTimeout  time.Duration
	parsedHeartbeatTimeout time.Duration
	parsedSnapshotInterval time.Duration
}

type GRPCConfig struct {
	ListenAddress string `mapstructure:"listen_address"` // Address:Port for the gRPC server
}

type SigningConfig struct {
	PrivateKeyPath string `mapstructure:"private_key_path"` // Path to the private key file
	Scheme         string `mapstructure:"scheme"`           // Algorithm: "ed25519" or "bls" (requires specific implementation)
}

// LoadConfig loads and parses the configuration from a YAML file.
func LoadConfig(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	// Set default values
	v.SetDefault("execution.enabled", false)
	v.SetDefault("execution.type", "noop") // Default to no-op if enabled but type not specified
	v.SetDefault("execution.timeout", "15s")
	v.SetDefault("raft.election_timeout", "1s")
	v.SetDefault("raft.heartbeat_timeout", "100ms")
	v.SetDefault("raft.snapshot_interval", "120s")
	v.SetDefault("raft.snapshot_threshold", 8192)
	v.SetDefault("signing.scheme", "ed25519") // Default signing scheme

	if err := v.ReadInConfig(); err != nil {
		// Allow the file not to exist if it's the default path?
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// It's an error other than 'file not found'
			return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
		}
		// If it's ConfigFileNotFoundError, log that defaults are used or proceed silently
		// For now, we proceed using only defaults if file not found.
		fmt.Printf("Config file '%s' not found, using defaults.\n", path) // Log this info
	}

	var cfg Config
	// Use a decode hook to parse durations and slices
	decodeHook := mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","), // Assumes peers are comma-separated if passed as a string
	)

	// Configure Viper to use mapstructure with the hook
	if err := v.Unmarshal(&cfg, viper.DecodeHook(decodeHook)); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Manually parse ALL duration strings AFTER unmarshal to catch invalid formats
	var err error
	cfg.Raft.parsedElectionTimeout, err = time.ParseDuration(cfg.Raft.ElectionTimeout)
	if err != nil {
		return nil, fmt.Errorf("invalid raft.election_timeout '%s': %w", cfg.Raft.ElectionTimeout, err)
	}
	cfg.Raft.parsedHeartbeatTimeout, err = time.ParseDuration(cfg.Raft.HeartbeatTimeout)
	if err != nil {
		return nil, fmt.Errorf("invalid raft.heartbeat_timeout '%s': %w", cfg.Raft.HeartbeatTimeout, err)
	}
	cfg.Raft.parsedSnapshotInterval, err = time.ParseDuration(cfg.Raft.SnapshotInterval)
	if err != nil {
		return nil, fmt.Errorf("invalid raft.snapshot_interval '%s': %w", cfg.Raft.SnapshotInterval, err)
	}
	// We parse execution timeout later, only if enabled or to get a default value

	// Additional validations
	if cfg.Node.ID == "" {
		// If not set by file or flag, could potentially generate one?
		// For now, require it.
		return nil, fmt.Errorf("node.id is required")
	}
	if cfg.Signing.PrivateKeyPath == "" {
		return nil, fmt.Errorf("signing.private_key_path is required")
	}
	if _, err := os.Stat(cfg.Signing.PrivateKeyPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("signing private key file not found: %s", cfg.Signing.PrivateKeyPath)
	}
	if cfg.Raft.DataDir == "" {
		return nil, fmt.Errorf("raft.data_dir is required")
	}

	// Execution config validation and parsing
	if cfg.Execution.Enabled {
		if cfg.Execution.Type == "" {
			return nil, fmt.Errorf("execution.type is required when execution.enabled is true")
		}
		if cfg.Execution.Type == "fullnode" && cfg.Execution.FullnodeEndpoint == "" {
			return nil, fmt.Errorf("execution.fullnode_endpoint is required when execution.type is 'fullnode'")
		}
		// Parse execution timeout explicitly here to catch errors
		// var err error // already declared above
		cfg.Execution.parsedTimeout, err = time.ParseDuration(cfg.Execution.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid execution.timeout '%s': %w", cfg.Execution.Timeout, err)
		}
		if cfg.Execution.parsedTimeout <= 0 {
			// This check might be redundant now but kept for safety
			return nil, fmt.Errorf("execution.timeout must be a positive duration, got '%s'", cfg.Execution.Timeout)
		}
	} else {
		// If not enabled, parse the default/provided timeout but don't error on negative/zero
		// Assign a default sensible value if parsing fails or is non-positive
		parsed, err := time.ParseDuration(cfg.Execution.Timeout)
		if err != nil || parsed <= 0 {
			// Use the default value set by viper earlier if parsing fails or result is non-positive
			defaultTimeoutStr := v.GetString("execution.timeout")
			parsedDefault, defaultErr := time.ParseDuration(defaultTimeoutStr)
			if defaultErr != nil {
				// Fallback to hardcoded default if the viper default itself is invalid
				cfg.Execution.parsedTimeout = 15 * time.Second
			} else {
				cfg.Execution.parsedTimeout = parsedDefault
			}
		} else {
			cfg.Execution.parsedTimeout = parsed
		}
	}

	return &cfg, nil
}

// Getters for parsed durations
func (rc *RaftConfig) GetElectionTimeout() time.Duration  { return rc.parsedElectionTimeout }
func (rc *RaftConfig) GetHeartbeatTimeout() time.Duration { return rc.parsedHeartbeatTimeout }
func (rc *RaftConfig) GetSnapshotInterval() time.Duration { return rc.parsedSnapshotInterval }

// Getter for parsed execution timeout
func (ec *ExecutionConfig) GetTimeout() time.Duration { return ec.parsedTimeout }
