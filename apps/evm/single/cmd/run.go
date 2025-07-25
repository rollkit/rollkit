package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/evstack/ev-node/da/jsonrpc"
	"github.com/evstack/ev-node/node"
	"github.com/evstack/ev-node/sequencers/single"

	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"

	"github.com/evstack/ev-node/execution/evm"

	"github.com/evstack/ev-node/core/execution"
	rollcmd "github.com/evstack/ev-node/pkg/cmd"
	"github.com/evstack/ev-node/pkg/config"
	"github.com/evstack/ev-node/pkg/p2p"
	"github.com/evstack/ev-node/pkg/p2p/key"
	"github.com/evstack/ev-node/pkg/store"
)

var RunCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"node", "run"},
	Short:   "Run the rollkit node with EVM execution client",
	RunE: func(cmd *cobra.Command, args []string) error {
		executor, err := createExecutionClient(cmd)
		if err != nil {
			return err
		}

		nodeConfig, err := rollcmd.ParseConfig(cmd)
		if err != nil {
			return err
		}

		logger := rollcmd.SetupLogger(nodeConfig.Log)

		daJrpc, err := jsonrpc.NewClient(context.Background(), logger, nodeConfig.DA.Address, nodeConfig.DA.AuthToken, nodeConfig.DA.Namespace)
		if err != nil {
			return err
		}

		datastore, err := store.NewDefaultKVStore(nodeConfig.RootDir, nodeConfig.DBPath, "evm-single")
		if err != nil {
			return err
		}

		singleMetrics, err := single.DefaultMetricsProvider(nodeConfig.Instrumentation.IsPrometheusEnabled())(nodeConfig.ChainID)
		if err != nil {
			return err
		}

		sequencer, err := single.NewSequencer(
			context.Background(),
			logger,
			datastore,
			&daJrpc.DA,
			[]byte(nodeConfig.ChainID),
			nodeConfig.Node.BlockTime.Duration,
			singleMetrics,
			nodeConfig.Node.Aggregator,
		)
		if err != nil {
			return err
		}

		nodeKey, err := key.LoadNodeKey(filepath.Dir(nodeConfig.ConfigPath()))
		if err != nil {
			return err
		}

		p2pClient, err := p2p.NewClient(nodeConfig, nodeKey, datastore, logger, nil)
		if err != nil {
			return err
		}

		return rollcmd.StartNode(logger, cmd, executor, sequencer, &daJrpc.DA, p2pClient, datastore, nodeConfig, node.NodeOptions{})
	},
}

func init() {
	config.AddFlags(RunCmd)
	addFlags(RunCmd)
}

func createExecutionClient(cmd *cobra.Command) (execution.Executor, error) {
	// Read execution client parameters from flags
	ethURL, err := cmd.Flags().GetString(evm.FlagEvmEthURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get '%s' flag: %w", evm.FlagEvmEthURL, err)
	}
	engineURL, err := cmd.Flags().GetString(evm.FlagEvmEngineURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get '%s' flag: %w", evm.FlagEvmEngineURL, err)
	}
	jwtSecret, err := cmd.Flags().GetString(evm.FlagEvmJWTSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to get '%s' flag: %w", evm.FlagEvmJWTSecret, err)
	}
	genesisHashStr, err := cmd.Flags().GetString(evm.FlagEvmGenesisHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get '%s' flag: %w", evm.FlagEvmGenesisHash, err)
	}
	feeRecipientStr, err := cmd.Flags().GetString(evm.FlagEvmFeeRecipient)
	if err != nil {
		return nil, fmt.Errorf("failed to get '%s' flag: %w", evm.FlagEvmFeeRecipient, err)
	}

	// Convert string parameters to Ethereum types
	genesisHash := common.HexToHash(genesisHashStr)
	feeRecipient := common.HexToAddress(feeRecipientStr)

	return evm.NewEngineExecutionClient(ethURL, engineURL, jwtSecret, genesisHash, feeRecipient)
}

// addFlags adds flags related to the EVM execution client
func addFlags(cmd *cobra.Command) {
	cmd.Flags().String(evm.FlagEvmEthURL, "http://localhost:8545", "URL of the Ethereum JSON-RPC endpoint")
	cmd.Flags().String(evm.FlagEvmEngineURL, "http://localhost:8551", "URL of the Engine API endpoint")
	cmd.Flags().String(evm.FlagEvmJWTSecret, "", "The JWT secret for authentication with the execution client")
	cmd.Flags().String(evm.FlagEvmGenesisHash, "", "Hash of the genesis block")
	cmd.Flags().String(evm.FlagEvmFeeRecipient, "", "Address that will receive transaction fees")
}
