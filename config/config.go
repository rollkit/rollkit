package config

import (
	"errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"time"
)


// NodeConfig stores Optimint node configuration.
type NodeConfig struct {
	RootDir            string `mapstructure:"home"`
	DBPath             string
	P2P                P2PConfig `mapstructure:"p2p"`
	Aggregator         bool      `mapstructure:"optimint.aggregator"`
	BlockManagerConfig `mapstructure:"optimint.block_manager"`
	DALayer            string `mapstructure:"optimint.da_layer"`
	DAConfig           string `mapstructure:"optimint.da_config"`
}

// BlockManagerConfig consists of all parameters required by BlockManagerConfig
type BlockManagerConfig struct {
	BlockTime   time.Duration `mapstructure:"block_time"`
	NamespaceID [8]byte       `mapstructure:"namespace_id"`
}

func ReadConfig(v *viper.Viper) *NodeConfig {
	var cfg NodeConfig

	err := v.Unmarshal(&cfg)
	if errors.Is(err, viper.ConfigFileNotFoundError{}) {

	}

	return &cfg
}

func AddFlags(cmd *cobra.Command, cfg *NodeConfig) {
	cmd.Flags().Bool("optimint.aggregator", cfg.Aggregator, "run node in aggregator mode")
	cmd.Flags().String("optimint.da_layer", cfg.DALayer, "Data Availability Layer Client name (mock or grpc")
	cmd.Flags().String("optimint.da_config", cfg.DAConfig, "Data Availability Layer Client config")
	cmd.Flags().Duration("optimint.block_time", cfg.BlockManagerConfig.BlockTime, "block time (for aggregator mode)")
	cmd.Flags().BytesHex("optimint.namespace_id", cfg.BlockManagerConfig.NamespaceID[:], "namespace identifies (8 bytes in hex)")
}