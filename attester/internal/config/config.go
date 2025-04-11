package config

import (
	"fmt"
	"os"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

type AggregatorConfig struct {
	QuorumThreshold int               `mapstructure:"quorum_threshold" yaml:"quorum_threshold"`
	Attesters       map[string]string `mapstructure:"attesters"        yaml:"attesters"`
}

type NetworkConfig struct {
	SequencerSigEndpoint string `mapstructure:"sequencer_sig_endpoint" yaml:"sequencer_sig_endpoint,omitempty"`
}

type ExecutionConfig struct {
	Enabled          bool          `mapstructure:"enabled"           yaml:"enabled"`
	Type             string        `mapstructure:"type"              yaml:"type"` // e.g., "noop", "fullnode"
	FullnodeEndpoint string        `mapstructure:"fullnode_endpoint" yaml:"fullnode_endpoint,omitempty"`
	Timeout          time.Duration `mapstructure:"timeout"           yaml:"timeout"` // Use time.Duration directly
}

type Config struct {
	Node       NodeConfig       `mapstructure:"node"       yaml:"node"`
	Raft       RaftConfig       `mapstructure:"raft"       yaml:"raft"`
	GRPC       GRPCConfig       `mapstructure:"grpc"       yaml:"grpc"`
	Signing    SigningConfig    `mapstructure:"signing"    yaml:"signing"`
	Network    NetworkConfig    `mapstructure:"network"    yaml:"network"`
	Aggregator AggregatorConfig `mapstructure:"aggregator" yaml:"aggregator"`
	Execution  ExecutionConfig  `mapstructure:"execution"  yaml:"execution"`
}

type NodeConfig struct {
	ID              string `mapstructure:"id"                yaml:"id"`                // Unique node ID within the RAFT cluster
	RaftBindAddress string `mapstructure:"raft_bind_address" yaml:"raft_bind_address"` // Address:Port for internal RAFT communication
}

type PeerConfig struct {
	ID      string `yaml:"id"`
	Address string `yaml:"address"`
}

type RaftConfig struct {
	DataDir           string        `mapstructure:"data_dir"           yaml:"data_dir"`           // Directory for logs, snapshots, stable store
	Peers             []PeerConfig  `mapstructure:"peers"              yaml:"peers,omitempty"`    // List of RAFT peer configurations
	BootstrapCluster  bool          `mapstructure:"bootstrap_cluster"  yaml:"bootstrap_cluster"`  // Should this node bootstrap a new cluster?
	ElectionTimeout   time.Duration `mapstructure:"election_timeout"   yaml:"election_timeout"`   // Use time.Duration directly
	HeartbeatTimeout  time.Duration `mapstructure:"heartbeat_timeout"  yaml:"heartbeat_timeout"`  // Use time.Duration directly
	SnapshotInterval  time.Duration `mapstructure:"snapshot_interval"  yaml:"snapshot_interval"`  // Use time.Duration directly
	SnapshotThreshold uint64        `mapstructure:"snapshot_threshold" yaml:"snapshot_threshold"` // Number of commits to trigger snapshot
}

type GRPCConfig struct {
	ListenAddress string `mapstructure:"listen_address" yaml:"listen_address,omitempty"` // Address:Port for the gRPC server
}

type SigningConfig struct {
	PrivateKeyPath string `mapstructure:"private_key_path" yaml:"private_key_path"` // Path to the private key file
	Scheme         string `mapstructure:"scheme"           yaml:"scheme"`           // Algorithm: "ed25519" or "bls"
}

func LoadConfig(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	// Set default values using duration strings, Viper/Mapstructure will handle parsing
	v.SetDefault("execution.enabled", false)
	v.SetDefault("execution.type", "noop")
	v.SetDefault("execution.timeout", "15s")
	v.SetDefault("raft.election_timeout", "1s")
	v.SetDefault("raft.heartbeat_timeout", "500ms")
	v.SetDefault("raft.snapshot_interval", "120s")
	v.SetDefault("raft.snapshot_threshold", 8192)
	v.SetDefault("signing.scheme", "ed25519")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
		}
		fmt.Printf("Config file '%s' not found, using defaults.\n", path) // Log this info
	}

	var cfg Config
	// Use a decode hook to parse time durations directly into time.Duration fields
	decodeHook := mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
	)

	// Configure Viper to use mapstructure with the hook
	if err := v.Unmarshal(&cfg, viper.DecodeHook(decodeHook)); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config (check duration formats): %w", err)
	}

	if cfg.Node.ID == "" {
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
	if cfg.Raft.ElectionTimeout <= 0 {
		return nil, fmt.Errorf("raft.election_timeout must be a positive duration")
	}
	if cfg.Raft.HeartbeatTimeout <= 0 {
		return nil, fmt.Errorf("raft.heartbeat_timeout must be a positive duration")
	}
	if cfg.Raft.SnapshotInterval <= 0 {
		return nil, fmt.Errorf("raft.snapshot_interval must be a positive duration")
	}

	if cfg.Execution.Enabled {
		if cfg.Execution.Type == "" {
			return nil, fmt.Errorf("execution.type is required when execution.enabled is true")
		}
		if cfg.Execution.Type == "fullnode" && cfg.Execution.FullnodeEndpoint == "" {
			return nil, fmt.Errorf("execution.fullnode_endpoint is required when execution.type is 'fullnode'")
		}
		// Validate execution timeout is positive (already parsed by hook)
		if cfg.Execution.Timeout <= 0 {
			return nil, fmt.Errorf("execution.timeout must be a positive duration when execution is enabled, got '%s'", cfg.Execution.Timeout)
		}
	} else {
		if cfg.Execution.Timeout <= 0 {
			fmt.Printf("Warning: execution.timeout ('%s') is non-positive but execution is disabled. Using the value.\n", cfg.Execution.Timeout)
		}
	}

	return &cfg, nil
}
