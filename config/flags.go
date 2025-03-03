package config

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	// FlagAggregator is a flag for running node in aggregator mode
	FlagAggregator = "rollkit.aggregator"
	// FlagDAAddress is a flag for specifying the data availability layer address
	FlagDAAddress = "rollkit.da_address"
	// FlagDAAuthToken is a flag for specifying the data availability layer auth token
	FlagDAAuthToken = "rollkit.da_auth_token" // #nosec G101
	// FlagBlockTime is a flag for specifying the block time
	FlagBlockTime = "rollkit.block_time"
	// FlagDABlockTime is a flag for specifying the data availability layer block time
	FlagDABlockTime = "rollkit.da_block_time"
	// FlagDAGasPrice is a flag for specifying the data availability layer gas price
	FlagDAGasPrice = "rollkit.da_gas_price"
	// FlagDAGasMultiplier is a flag for specifying the data availability layer gas price retry multiplier
	FlagDAGasMultiplier = "rollkit.da_gas_multiplier"
	// FlagDAStartHeight is a flag for specifying the data availability layer start height
	FlagDAStartHeight = "rollkit.da_start_height"
	// FlagDANamespace is a flag for specifying the DA namespace ID
	FlagDANamespace = "rollkit.da_namespace"
	// FlagDASubmitOptions is a flag for data availability submit options
	FlagDASubmitOptions = "rollkit.da_submit_options"
	// FlagLight is a flag for running the node in light mode
	FlagLight = "rollkit.light"
	// FlagTrustedHash is a flag for specifying the trusted hash
	FlagTrustedHash = "rollkit.trusted_hash"
	// FlagLazyAggregator is a flag for enabling lazy aggregation
	FlagLazyAggregator = "rollkit.lazy_aggregator"
	// FlagMaxPendingBlocks is a flag to pause aggregator in case of large number of blocks pending DA submission
	FlagMaxPendingBlocks = "rollkit.max_pending_blocks"
	// FlagDAMempoolTTL is a flag for specifying the DA mempool TTL
	FlagDAMempoolTTL = "rollkit.da_mempool_ttl"
	// FlagLazyBlockTime is a flag for specifying the block time in lazy mode
	FlagLazyBlockTime = "rollkit.lazy_block_time"
	// FlagSequencerAddress is a flag for specifying the sequencer middleware address
	FlagSequencerAddress = "rollkit.sequencer_address"
	// FlagSequencerRollupID is a flag for specifying the sequencer middleware rollup ID
	FlagSequencerRollupID = "rollkit.sequencer_rollup_id"
	// FlagExecutorAddress is a flag for specifying the sequencer middleware address
	FlagExecutorAddress = "rollkit.executor_address"
)

// GetViperConfig reads configuration parameters from Viper instance.
//
// This method is called in cosmos-sdk.
func (nc *NodeConfig) GetViperConfig(v *viper.Viper) error {
	nc.Aggregator = v.GetBool(FlagAggregator)
	nc.DataAvailability.AuthToken = v.GetString(FlagDAAuthToken)
	nc.DataAvailability.GasPrice = v.GetFloat64(FlagDAGasPrice)
	nc.DataAvailability.GasMultiplier = v.GetFloat64(FlagDAGasMultiplier)
	nc.DataAvailability.Namespace = v.GetString(FlagDANamespace)
	nc.DAStartHeight = v.GetUint64(FlagDAStartHeight)
	nc.BlockTime = v.GetDuration(FlagBlockTime)
	nc.LazyAggregator = v.GetBool(FlagLazyAggregator)
	nc.Light = v.GetBool(FlagLight)
	nc.TrustedHash = v.GetString(FlagTrustedHash)
	nc.MaxPendingBlocks = v.GetUint64(FlagMaxPendingBlocks)
	nc.DAMempoolTTL = v.GetUint64(FlagDAMempoolTTL)
	nc.LazyBlockTime = v.GetDuration(FlagLazyBlockTime)
	nc.Sequencer.RollupID = v.GetString(FlagSequencerRollupID)
	nc.ExecutorAddress = v.GetString(FlagExecutorAddress)

	return nil
}

// AddFlags adds Rollkit specific configuration options to cobra Command.
//
// This function is called in cosmos-sdk.
func AddFlags(cmd *cobra.Command) {
	def := DefaultNodeConfig

	cmd.Flags().BoolVar(&def.Aggregator, FlagAggregator, def.Aggregator, "run node in aggregator mode")
	cmd.Flags().Bool(FlagLazyAggregator, def.LazyAggregator, "wait for transactions, don't build empty blocks")
	cmd.Flags().String(FlagDAAuthToken, def.DataAvailability.AuthToken, "DA auth token")
	cmd.Flags().Duration(FlagBlockTime, def.BlockTime, "block time (for aggregator mode)")
	cmd.Flags().Duration(FlagDABlockTime, def.BlockTime, "DA chain block time (for syncing)")
	cmd.Flags().Float64(FlagDAGasPrice, def.DataAvailability.GasPrice, "DA gas price for blob transactions")
	cmd.Flags().Float64(FlagDAGasMultiplier, def.DataAvailability.GasMultiplier, "DA gas price multiplier for retrying blob transactions")
	cmd.Flags().Uint64(FlagDAStartHeight, def.DAStartHeight, "starting DA block height (for syncing)")
	cmd.Flags().String(FlagDANamespace, def.DataAvailability.Namespace, "DA namespace to submit blob transactions")
	cmd.Flags().String(FlagDASubmitOptions, def.DataAvailability.SubmitOptions, "DA submit options")
	cmd.Flags().Bool(FlagLight, def.Light, "run light client")
	cmd.Flags().String(FlagTrustedHash, def.TrustedHash, "initial trusted hash to start the header exchange service")
	cmd.Flags().Uint64(FlagMaxPendingBlocks, def.MaxPendingBlocks, "limit of blocks pending DA submission (0 for no limit)")
	cmd.Flags().Uint64(FlagDAMempoolTTL, def.DAMempoolTTL, "number of DA blocks until transaction is dropped from the mempool")
	cmd.Flags().Duration(FlagLazyBlockTime, def.LazyBlockTime, "block time (for lazy mode)")
	cmd.Flags().String(FlagSequencerRollupID, def.Sequencer.RollupID, "sequencer middleware rollup ID (default: mock-rollup)")
	cmd.Flags().String(FlagExecutorAddress, def.ExecutorAddress, "executor middleware address (host:port)")
}
