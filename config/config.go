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
	nc.Aggregator = v.GetBool("optimint.aggregator")
	nc.DALayer = v.GetString("optimint.da_layer")
	nc.DAConfig = v.GetString("optimint.da_config")
	nc.BlockTime = v.GetDuration("optimint.block_time")
	nsID := v.GetString("optimint.namespace_id")
	copy(nc.NamespaceID[:], nsID)
}
