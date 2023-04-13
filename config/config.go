package config

import (
	"encoding/hex"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rollkit/rollkit/types"
)

const (
	FlagAggregator     = "rollkit.aggregator"
	FlagDALayer        = "rollkit.da_layer"
	FlagDAConfig       = "rollkit.da_config"
	FlagBlockTime      = "rollkit.block_time"
	FlagDABlockTime    = "rollkit.da_block_time"
	FlagDAStartHeight  = "rollkit.da_start_height"
	FlagNamespaceID    = "rollkit.namespace_id"
	FlagFraudProofs    = "rollkit.experimental_insecure_fraud_proofs"
	FlagLight          = "rollkit.light"
	FlagTrustedHash    = "rollkit.trusted_hash"
	FlagLazyAggregator = "rollkit.lazy_aggregator"
)

// NodeConfig stores Rollkit node configuration.
type NodeConfig struct {
	// parameters below are translated from existing config
	RootDir string
	DBPath  string
	P2P     P2PConfig
	RPC     RPCConfig
	// parameters below are Rollkit specific and read from config
	Aggregator         bool `mapstructure:"aggregator"`
	BlockManagerConfig `mapstructure:",squash"`
	DALayer            string `mapstructure:"da_layer"`
	DAConfig           string `mapstructure:"da_config"`
	Light              bool   `mapstructure:"light"`
	HeaderConfig       `mapstructure:",squash"`
	LazyAggregator     bool `mapstructure:"lazy_aggregator"`
}

// HeaderConfig allows node to pass the initial trusted header hash to start the header exchange service
type HeaderConfig struct {
	TrustedHash string `mapstructure:"trusted_hash"`
}

// BlockManagerConfig consists of all parameters required by BlockManagerConfig
type BlockManagerConfig struct {
	// BlockTime defines how often new blocks are produced
	BlockTime time.Duration `mapstructure:"block_time"`
	// DABlockTime informs about block time of underlying data availability layer
	DABlockTime time.Duration `mapstructure:"da_block_time"`
	// DAStartHeight allows skipping first DAStartHeight-1 blocks when querying for blocks.
	DAStartHeight uint64            `mapstructure:"da_start_height"`
	NamespaceID   types.NamespaceID `mapstructure:"namespace_id"`
	FraudProofs   bool              `mapstructure:"fraud_proofs"`
}

// GetViperConfig reads configuration parameters from Viper instance.
//
// This method is called in cosmos-sdk.
func (nc *NodeConfig) GetViperConfig(v *viper.Viper) error {
	nc.Aggregator = v.GetBool(FlagAggregator)
	nc.DALayer = v.GetString(FlagDALayer)
	nc.DAConfig = v.GetString(FlagDAConfig)
	nc.DAStartHeight = v.GetUint64(FlagDAStartHeight)
	nc.DABlockTime = v.GetDuration(FlagDABlockTime)
	nc.BlockTime = v.GetDuration(FlagBlockTime)
	nc.LazyAggregator = v.GetBool(FlagLazyAggregator)
	nsID := v.GetString(FlagNamespaceID)
	nc.FraudProofs = v.GetBool(FlagFraudProofs)
	nc.Light = v.GetBool(FlagLight)
	bytes, err := hex.DecodeString(nsID)
	if err != nil {
		return err
	}
	copy(nc.NamespaceID[:], bytes)
	nc.TrustedHash = v.GetString(FlagTrustedHash)
	return nil
}

// AddFlags adds Rollkit specific configuration options to cobra Command.
//
// This function is called in cosmos-sdk.
func AddFlags(cmd *cobra.Command) {
	def := DefaultNodeConfig
	cmd.Flags().Bool(FlagAggregator, def.Aggregator, "run node in aggregator mode")
	cmd.Flags().Bool(FlagLazyAggregator, def.LazyAggregator, "wait for transactions, don't build empty blocks")
	cmd.Flags().String(FlagDALayer, def.DALayer, "Data Availability Layer Client name (mock or grpc")
	cmd.Flags().String(FlagDAConfig, def.DAConfig, "Data Availability Layer Client config")
	cmd.Flags().Duration(FlagBlockTime, def.BlockTime, "block time (for aggregator mode)")
	cmd.Flags().Duration(FlagDABlockTime, def.DABlockTime, "DA chain block time (for syncing)")
	cmd.Flags().Uint64(FlagDAStartHeight, def.DAStartHeight, "starting DA block height (for syncing)")
	cmd.Flags().BytesHex(FlagNamespaceID, def.NamespaceID[:], "namespace identifies (8 bytes in hex)")
	cmd.Flags().Bool(FlagFraudProofs, def.FraudProofs, "enable fraud proofs (experimental & insecure)")
	cmd.Flags().Bool(FlagLight, def.Light, "run light client")
	cmd.Flags().String(FlagTrustedHash, def.TrustedHash, "initial trusted hash to start the header exchange service")
}
