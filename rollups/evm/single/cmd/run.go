package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/rollkit/rollkit/sequencers/single"
	"github.com/rs/zerolog"

	"cosmossdk.io/log"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"

	evm "github.com/rollkit/go-execution-evm"

	"github.com/rollkit/rollkit/core/execution"
	"github.com/rollkit/rollkit/da/proxy"
	rollcmd "github.com/rollkit/rollkit/pkg/cmd"
	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/p2p"
	"github.com/rollkit/rollkit/pkg/p2p/key"
	"github.com/rollkit/rollkit/pkg/store"
)

var RunCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"node", "run"},
	Short:   "Run the rollkit node with EVM execution client",
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := []log.Option{}
		logLevel, _ := cmd.Flags().GetString(config.FlagLogLevel)
		if logLevel != "" {
			zl, err := zerolog.ParseLevel(logLevel)
			if err != nil {
				return err
			}
			opts = append(opts, log.LevelOption(zl))
		}

		executor, err := createExecutionClient(cmd)
		if err != nil {
			return err
		}

		nodeConfig, err := rollcmd.ParseConfig(cmd)
		if err != nil {
			return err
		}

		logger := rollcmd.SetupLogger(nodeConfig.Log)

		daJrpc, err := proxy.NewClient(logger, nodeConfig.DA.Address, nodeConfig.DA.AuthToken)
		if err != nil {
			return err
		}

		datastore, err := store.NewDefaultKVStore(nodeConfig.RootDir, nodeConfig.DBPath, "evm-single")
		if err != nil {
			return err
		}

		singleMetrics, err := single.NopMetrics()
		if err != nil {
			return err
		}

		sequencer, err := single.NewSequencer(
			context.Background(),
			logger,
			datastore,
			daJrpc,
			[]byte(nodeConfig.DA.Namespace),
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

		return rollcmd.StartNode(logger, cmd, executor, sequencer, daJrpc, []byte(nodeConfig.DA.Namespace), nodeKey, p2pClient, datastore, nodeConfig)
	},
}

func init() {
	config.AddFlags(RunCmd)
	addFlags(RunCmd)
}

func createExecutionClient(cmd *cobra.Command) (execution.Executor, error) {
	// Read execution client parameters from flags
	ethURL, err := cmd.Flags().GetString("evm.eth-url")
	if err != nil {
		return nil, fmt.Errorf("failed to get 'evm.eth-url' flag: %w", err)
	}
	engineURL, err := cmd.Flags().GetString("evm.engine-url")
	if err != nil {
		return nil, fmt.Errorf("failed to get 'evm.engine-url' flag: %w", err)
	}
	jwtSecret, err := cmd.Flags().GetString("evm.jwt-secret")
	if err != nil {
		return nil, fmt.Errorf("failed to get 'evm.jwt-secret' flag: %w", err)
	}
	genesisHashStr, err := cmd.Flags().GetString("evm.genesis-hash")
	if err != nil {
		return nil, fmt.Errorf("failed to get 'evm.genesis-hash' flag: %w", err)
	}
	feeRecipientStr, err := cmd.Flags().GetString("evm.fee-recipient")
	if err != nil {
		return nil, fmt.Errorf("failed to get 'evm.fee-recipient' flag: %w", err)
	}

	// Convert string parameters to Ethereum types
	genesisHash := common.HexToHash(genesisHashStr)
	feeRecipient := common.HexToAddress(feeRecipientStr)

	return evm.NewPureEngineExecutionClient(ethURL, engineURL, jwtSecret, genesisHash, feeRecipient)
}

// addFlags adds flags related to the EVM execution client
func addFlags(cmd *cobra.Command) {
	cmd.Flags().String("evm.eth-url", "http://localhost:8545", "URL of the Ethereum JSON-RPC endpoint")
	cmd.Flags().String("evm.engine-url", "http://localhost:8551", "URL of the Engine API endpoint")
	cmd.Flags().String("evm.jwt-secret", "", "Path to the JWT secret file for authentication with the execution client")
	cmd.Flags().String("evm.genesis-hash", "", "Hash of the genesis block")
	cmd.Flags().String("evm.fee-recipient", "", "Address that will receive transaction fees")
}
