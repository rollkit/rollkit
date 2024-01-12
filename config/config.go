package config

import (
	"time"

	cmcfg "github.com/cometbft/cometbft/config"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	flagAggregator     = "rollkit.aggregator"
	flagDAAddress      = "rollkit.da_address"
	flagBlockTime      = "rollkit.block_time"
	flagDABlockTime    = "rollkit.da_block_time"
	flagDAStartHeight  = "rollkit.da_start_height"
	flagLight          = "rollkit.light"
	flagTrustedHash    = "rollkit.trusted_hash"
	flagLazyAggregator = "rollkit.lazy_aggregator"
	flagDAGasPrice     = "rollkit.da_gas_price"
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
	DAAddress          string `mapstructure:"da_address"`
	Light              bool   `mapstructure:"light"`
	HeaderConfig       `mapstructure:",squash"`
	LazyAggregator     bool    `mapstructure:"lazy_aggregator"`
	DAGasPrice         float64 `mapstructure:"da_gas_price"`
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
	DAStartHeight uint64 `mapstructure:"da_start_height"`
}

// GetNodeConfig translates Tendermint's configuration into Rollkit configuration.
//
// This method only translates configuration, and doesn't verify it. If some option is missing in Tendermint's
// config, it's skipped during translation.
func GetNodeConfig(nodeConf *NodeConfig, cmConf *cmcfg.Config) {
	if cmConf != nil {
		nodeConf.RootDir = cmConf.RootDir
		nodeConf.DBPath = cmConf.DBPath
		if cmConf.P2P != nil {
			nodeConf.P2P.ListenAddress = cmConf.P2P.ListenAddress
			nodeConf.P2P.Seeds = cmConf.P2P.Seeds
		}
		if cmConf.RPC != nil {
			nodeConf.RPC.ListenAddress = cmConf.RPC.ListenAddress
			nodeConf.RPC.CORSAllowedOrigins = cmConf.RPC.CORSAllowedOrigins
			nodeConf.RPC.CORSAllowedMethods = cmConf.RPC.CORSAllowedMethods
			nodeConf.RPC.CORSAllowedHeaders = cmConf.RPC.CORSAllowedHeaders
			nodeConf.RPC.MaxOpenConnections = cmConf.RPC.MaxOpenConnections
			nodeConf.RPC.TLSCertFile = cmConf.RPC.TLSCertFile
			nodeConf.RPC.TLSKeyFile = cmConf.RPC.TLSKeyFile
		}
	}
}

// GetViperConfig reads configuration parameters from Viper instance.
//
// This method is called in cosmos-sdk.
func (nc *NodeConfig) GetViperConfig(v *viper.Viper) error {
	nc.Aggregator = v.GetBool(flagAggregator)
	nc.DAAddress = v.GetString(flagDAAddress)
	nc.DAStartHeight = v.GetUint64(flagDAStartHeight)
	nc.DABlockTime = v.GetDuration(flagDABlockTime)
	nc.BlockTime = v.GetDuration(flagBlockTime)
	nc.LazyAggregator = v.GetBool(flagLazyAggregator)
	nc.Light = v.GetBool(flagLight)
	nc.TrustedHash = v.GetString(flagTrustedHash)
	nc.DAGasPrice = v.GetFloat64(flagDAGasPrice)
	return nil
}

// AddFlags adds Rollkit specific configuration options to cobra Command.
//
// This function is called in cosmos-sdk.
func AddFlags(cmd *cobra.Command) {
	def := DefaultNodeConfig
	cmd.Flags().Bool(flagAggregator, def.Aggregator, "run node in aggregator mode")
	cmd.Flags().Bool(flagLazyAggregator, def.LazyAggregator, "wait for transactions, don't build empty blocks")
	cmd.Flags().String(flagDAAddress, def.DAAddress, "DA address (host:port)")
	cmd.Flags().Duration(flagBlockTime, def.BlockTime, "block time (for aggregator mode)")
	cmd.Flags().Duration(flagDABlockTime, def.DABlockTime, "DA chain block time (for syncing)")
	cmd.Flags().Float64(flagDAGasPrice, def.DAGasPrice, "DA gas price for blob transactions")
	cmd.Flags().Uint64(flagDAStartHeight, def.DAStartHeight, "starting DA block height (for syncing)")
	cmd.Flags().Bool(flagLight, def.Light, "run light client")
	cmd.Flags().String(flagTrustedHash, def.TrustedHash, "initial trusted hash to start the header exchange service")
}
