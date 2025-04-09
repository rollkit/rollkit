package main

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	evm "github.com/rollkit/go-execution-evm"
	"github.com/rollkit/rollkit/core/execution"
	"os"

	"cosmossdk.io/log"
	"github.com/spf13/cobra"

	coreda "github.com/rollkit/rollkit/core/da"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/da"
	rollcmd "github.com/rollkit/rollkit/pkg/cmd"
	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/p2p"
	"github.com/rollkit/rollkit/pkg/p2p/key"
	"github.com/rollkit/rollkit/pkg/store"
)

var RunCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"node", "run"},
	Short:   "Run the rollkit node",
	RunE: func(cmd *cobra.Command, args []string) error {

		// Create test implementations
		// TODO: we need to start the executor http server
		executor, err := createExecutionClient(cmd)
		if err != nil {
			panic(err)
		}
		sequencer := coresequencer.NewDummySequencer()

		nodeConfig, err := rollcmd.ParseConfig(cmd, cmd.Flag(config.FlagRootDir).Value.String())
		if err != nil {
			panic(err)
		}

		// Create DA client with dummy DA
		dummyDA := coreda.NewDummyDA(100_000, 0, 0)
		logger := log.NewLogger(os.Stdout)
		dac := da.NewDAClient(dummyDA, 0, 1.0, []byte("test"), []byte(""), logger)

		nodeKey, err := key.LoadOrGenNodeKey(nodeConfig.ConfigDir)
		if err != nil {
			panic(err)
		}

		datastore, err := store.NewDefaultInMemoryKVStore()
		if err != nil {
			panic(err)
		}

		p2pClient, err := p2p.NewClient(config.DefaultNodeConfig, "testapp", nodeKey, datastore, logger, nil)
		if err != nil {
			panic(err)
		}

		return rollcmd.StartNode(cmd, executor, sequencer, dac, nodeKey, p2pClient, datastore)
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
