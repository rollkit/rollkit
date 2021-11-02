package config

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"time"
)

const (
	flagAggregator  = "optimint.aggregator"
	flagDALayer     = "optimint.da_layer"
	flagDAConfig    = "optimint.da_config"
	flagBlockTime   = "optimint.block_time"
	flagNamespaceID = "optimint.namespace_id"
)

// NodeConfig stores Optimint node configuration.
type NodeConfig struct {
	// parameters below are translated from existing config
	RootDir string
	DBPath  string
	P2P     P2PConfig
	// parameters below are optimint specific and read from config
	Aggregator         bool `mapstructure:"aggregator"`
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
	nc.Aggregator = v.GetBool(flagAggregator)
	nc.DALayer = v.GetString(flagDALayer)
	nc.DAConfig = v.GetString(flagDAConfig)
	nc.BlockTime = v.GetDuration(flagBlockTime)
	nsID := v.GetString(flagNamespaceID)
	copy(nc.NamespaceID[:], nsID)
}

func AddFlags(cmd *cobra.Command) {
	// TODO(tzdybal): extract default values
	cmd.Flags().Bool(flagAggregator, false, "run node in aggregator mode")
	cmd.Flags().String(flagDALayer, "mock", "Data Availability Layer Client name (mock or grpc")
	cmd.Flags().String(flagDAConfig, "", "Data Availability Layer Client config")
	cmd.Flags().Duration(flagBlockTime, 15*time.Second, "block time (for aggregator mode)")
	cmd.Flags().BytesHex(flagNamespaceID, nil, "namespace identifies (8 bytes in hex)")
}
