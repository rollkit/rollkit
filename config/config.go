package config

import (
	"github.com/spf13/viper"
	"time"
)


// NodeConfig stores Optimint node configuration.
type NodeConfig struct {
	// parameters below are translated from existing config
	RootDir            string
	DBPath             string
	P2P                P2PConfig
	// parameters below are optimint specific and read from config
	Aggregator         bool      `mapstructure:"aggregator"`
	BlockManagerConfig `mapstructure:",squash"`
	DALayer            string `mapstructure:"da_layer"`
	DAConfig           string `mapstructure:"da_config"`
}

// BlockManagerConfig consists of all parameters required by BlockManagerConfig
type BlockManagerConfig struct {
	BlockTime   time.Duration `mapstructure:"block_time"`
	NamespaceID [8]byte       `mapstructure:"namespace_id"`
}

func (nc *NodeConfig) GetViperConfig(v *viper.Viper) {
	_ =  v.UnmarshalKey("optimint", nc)
}

/*
func AddFlags(cmd *cobra.Command) {
	// TODO(tzdybal): unhardcode
	cmd.Flags().Bool("optimint.aggregator", false, "run node in aggregator mode")
	cmd.Flags().String("optimint.da_layer", "mock", "Data Availability Layer Client name (mock or grpc")
	cmd.Flags().String("optimint.da_config", "", "Data Availability Layer Client config")
	cmd.Flags().Duration("optimint.block_time", 15*time.Second, "block time (for aggregator mode)")
	cmd.Flags().BytesHex("optimint.namespace_id", nil, "namespace identifies (8 bytes in hex)")
}
 */