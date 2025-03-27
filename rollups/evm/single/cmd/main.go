package main

import (
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"

	evm "github.com/rollkit/go-execution-evm"
	cmd "github.com/rollkit/rollkit/cmd/rollkit/commands"
	"github.com/rollkit/rollkit/core/execution"
)

func main() {
	// Initiate the root command
	rootCmd := cmd.RootCmd

	// Add subcommands to the root command
	rootCmd.AddCommand(
		cmd.NewRunNodeCmd(createExecutionClient, addFlags),
	)

	if err := rootCmd.Execute(); err != nil {
		// Print to stderr and exit with error
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
func addFlags(cmd *cobra.Command) error {
	cmd.Flags().Bool("ci", false, "run node for ci testing")
	cmd.Flags().String("evm.eth-url", "http://localhost:8545", "URL of the Ethereum JSON-RPC endpoint")
	cmd.Flags().String("evm.engine-url", "http://localhost:8551", "URL of the Engine API endpoint")
	cmd.Flags().String("evm.jwt-secret", "", "Path to the JWT secret file for authentication with the execution client")
	cmd.Flags().String("evm.genesis-hash", "", "Hash of the genesis block")
	cmd.Flags().String("evm.fee-recipient", "", "Address that will receive transaction fees")
	return nil
}
