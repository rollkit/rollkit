package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	flagAggregator    = "optimint.aggregator"
	flagDALayer       = "optimint.da_layer"
	flagDAConfig      = "optimint.da_config"
	flagBlockTime     = "optimint.block_time"
	flagDABlockTime   = "optimint.da_block_time"
	flagDAStartHeight = "optimint.da_start_height"
	flagNamespaceID   = "optimint.namespace_id"
)

// NodeConfig stores Optimint node configuration.
type NodeConfig struct {
	// parameters below are translated from existing config
	RootDir string
	DBPath  string
	P2P     P2PConfig
	RPC     RPCConfig
	// parameters below are optimint specific and read from config
	Aggregator         bool `mapstructure:"aggregator"`
	BlockManagerConfig `mapstructure:",squash"`
	DALayer            string `mapstructure:"da_layer"`
	DAConfig           string `mapstructure:"da_config"`
}

// BlockManagerConfig consists of all parameters required by BlockManagerConfig
type BlockManagerConfig struct {
	// BlockTime defines how often new blocks are produced
	BlockTime time.Duration `mapstructure:"block_time"`
	// DABlockTime informs about block time of underlying data availability layer
	DABlockTime time.Duration `mapstructure:"da_block_time"`
	// DAStartHeight allows skipping first DAStartHeight-1 blocks when querying for blocks.
	DAStartHeight uint64  `mapstructure:"da_start_height"`
	NamespaceID   [8]byte `mapstructure:"namespace_id"`
}

// GetViperConfig reads configuration parameters from Viper instance.
//
// This method is called in cosmos-sdk.
func (nc *NodeConfig) GetViperConfig(v *viper.Viper) error {
	nc.Aggregator = v.GetBool(flagAggregator)
	nc.DALayer = v.GetString(flagDALayer)
	nc.DAConfig = v.GetString(flagDAConfig)
	nc.DAStartHeight = v.GetUint64(flagDAStartHeight)
	nc.DABlockTime = v.GetDuration(flagDABlockTime)
	nc.BlockTime = v.GetDuration(flagBlockTime)
	nsID := v.GetString(flagNamespaceID)
	// if namespace ID was not provided, we generate random one, and notify set it back to Viper instance
	switch nsID {
	case "":
		n, err := rand.Read(nc.NamespaceID[:])
		if err != nil || n != 8 {
			return fmt.Errorf("failed to generate random namespace ID: %w", err)
		}
		v.Set(flagNamespaceID, hex.EncodeToString(nc.NamespaceID[:]))
	default:
		bytes, err := hex.DecodeString(nsID)
		if err != nil {
			return err
		}
		copy(nc.NamespaceID[:], bytes)
	}
	return nil
}

// AddFlags adds Optimint specific configuration options to cobra Command.
//
// This function is called in cosmos-sdk.
func AddFlags(cmd *cobra.Command) {
	def := DefaultNodeConfig
	cmd.Flags().Bool(flagAggregator, def.Aggregator, "run node in aggregator mode")
	cmd.Flags().String(flagDALayer, def.DALayer, "Data Availability Layer Client name (mock or grpc")
	cmd.Flags().String(flagDAConfig, def.DAConfig, "Data Availability Layer Client config")
	cmd.Flags().Duration(flagBlockTime, def.BlockTime, "block time (for aggregator mode)")
	cmd.Flags().Duration(flagDABlockTime, def.DABlockTime, "DA chain block time (for syncing)")
	cmd.Flags().Uint64(flagDAStartHeight, def.DAStartHeight, "starting DA block height (for syncing)")
	cmd.Flags().BytesHex(flagNamespaceID, def.NamespaceID[:],
		"namespace identifies (8 bytes in hex), namespace ID will be randomized if not specified")
}
