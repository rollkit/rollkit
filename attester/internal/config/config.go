package config

import (
	"fmt"
	"os"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

type Config struct {
	Node    NodeConfig    `mapstructure:"node"`
	Raft    RaftConfig    `mapstructure:"raft"`
	GRPC    GRPCConfig    `mapstructure:"grpc"`
	Signing SigningConfig `mapstructure:"signing"`
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

	if err := v.ReadInConfig(); err != nil {
		// Allow the file not to exist if it's the default path?
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// It's an error other than 'file not found'
			return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
		}
		// If it's ConfigFileNotFoundError, we could continue with defaults or return a specific error.
		// For now, return error if not found.
		return nil, fmt.Errorf("config file not found at %s: %w", path, err)
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

	// Manually parse RaftConfig durations
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

	// Additional validations
	if cfg.Node.ID == "" {
		return nil, fmt.Errorf("node.id is required")
	}
	if cfg.Signing.PrivateKeyPath == "" {
		return nil, fmt.Errorf("signing.private_key_path is required")
	}
	if _, err := os.Stat(cfg.Signing.PrivateKeyPath); os.IsNotExist(err) {
		// Should we perhaps generate a key if it doesn't exist?
		return nil, fmt.Errorf("signing private key file not found: %s", cfg.Signing.PrivateKeyPath)
	}
	if cfg.Raft.DataDir == "" {
		return nil, fmt.Errorf("raft.data_dir is required")
	}

	return &cfg, nil
}

// Getters for parsed durations
func (rc *RaftConfig) GetElectionTimeout() time.Duration  { return rc.parsedElectionTimeout }
func (rc *RaftConfig) GetHeartbeatTimeout() time.Duration { return rc.parsedHeartbeatTimeout }
func (rc *RaftConfig) GetSnapshotInterval() time.Duration { return rc.parsedSnapshotInterval }
