package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/rollkit/go-execution-evm"
	coreda "github.com/rollkit/rollkit/core/da"

	"github.com/rollkit/rollkit/da/jsonrpc"
	rollcmd "github.com/rollkit/rollkit/pkg/cmd"
	rollconf "github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/p2p"
	"github.com/rollkit/rollkit/pkg/p2p/key"
	"github.com/rollkit/rollkit/pkg/store"
	"github.com/rollkit/rollkit/sequencers/based"
	"github.com/spf13/cobra"
)

func NewExtendedRunNodeCmd(ctx context.Context) *cobra.Command {
	var (
		ethURL              string
		engineURL           string
		jwtSecret           string
		genesisHash         string
		feeRecipient        string
		basedURL            string
		basedAuth           string
		basedNamespace      string
		basedStartHeight    uint64
		basedMaxHeightDrift uint64
		basedGasMultiplier  float64
		basedGasPrice       float64
	)

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Run the rollkit node in based mode",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			var err error

			ethURL, err = cmd.Flags().GetString("evm.eth-url")
			if err != nil {
				return fmt.Errorf("failed to get 'evm.eth-url' flag: %w", err)
			}
			engineURL, err = cmd.Flags().GetString("evm.engine-url")
			if err != nil {
				return fmt.Errorf("failed to get 'evm.engine-url' flag: %w", err)
			}
			jwtSecret, err = cmd.Flags().GetString("evm.jwt-secret")
			if err != nil {
				return fmt.Errorf("failed to get 'evm.jwt-secret' flag: %w", err)
			}
			genesisHash, err = cmd.Flags().GetString("evm.genesis-hash")
			if err != nil {
				return fmt.Errorf("failed to get 'evm.genesis-hash' flag: %w", err)
			}
			feeRecipient, err = cmd.Flags().GetString("evm.fee-recipient")
			if err != nil {
				return fmt.Errorf("failed to get 'evm.fee-recipient' flag: %w", err)
			}

			basedURL, err = cmd.Flags().GetString(based.FlagBasedURL)
			if err != nil {
				return fmt.Errorf("failed to get '%s' flag: %w", based.FlagBasedURL, err)
			}
			basedAuth, err = cmd.Flags().GetString(based.FlagBasedAuth)
			if err != nil {
				return fmt.Errorf("failed to get '%s' flag: %w", based.FlagBasedAuth, err)
			}
			basedNamespace, err = cmd.Flags().GetString(based.FlagBasedNamespace)
			if err != nil {
				return fmt.Errorf("failed to get '%s' flag: %w", based.FlagBasedNamespace, err)
			}
			basedStartHeight, err = cmd.Flags().GetUint64(based.FlagBasedStartHeight)
			if err != nil {
				return fmt.Errorf("failed to get '%s' flag: %w", based.FlagBasedStartHeight, err)
			}
			basedMaxHeightDrift, err = cmd.Flags().GetUint64(based.FlagBasedMaxHeightDrift)
			if err != nil {
				return fmt.Errorf("failed to get '%s' flag: %w", based.FlagBasedMaxHeightDrift, err)
			}
			basedGasMultiplier, err = cmd.Flags().GetFloat64(based.FlagBasedGasMultiplier)
			if err != nil {
				return fmt.Errorf("failed to get '%s' flag: %w", based.FlagBasedGasMultiplier, err)
			}
			basedGasPrice, err = cmd.Flags().GetFloat64(based.FlagBasedGasPrice)
			if err != nil {
				return fmt.Errorf("failed to get '%s' flag: %w", based.FlagBasedGasPrice, err)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {

			nodeConfig, err := rollcmd.ParseConfig(cmd)
			if err != nil {
				return fmt.Errorf("failed to parse config: %w", err)
			}

			executor, err := execution.NewEngineExecutionClient(
				ethURL, engineURL, jwtSecret, common.HexToHash(genesisHash), common.HexToAddress(feeRecipient),
			)
			if err != nil {
				return fmt.Errorf("failed to create execution client: %w", err)
			}

			logger := rollcmd.SetupLogger(nodeConfig.Log)

			var rollDA coreda.DA
			if nodeConfig.DA.AuthToken != "" {
				client, err := jsonrpc.NewClient(ctx, logger, nodeConfig.DA.Address, nodeConfig.DA.AuthToken, nodeConfig.DA.Namespace)
				if err != nil {
					return fmt.Errorf("failed to create DA client: %w", err)
				}
				rollDA = &client.DA
			} else {
				rollDA = coreda.NewDummyDA(100_000, 0, 0)
			}

			var basedDA coreda.DA
			if basedAuth != "" {
				client, err := jsonrpc.NewClient(ctx, logger, basedURL, basedAuth, basedNamespace)
				if err != nil {
					return fmt.Errorf("failed to create based client: %w", err)
				}
				basedDA = &client.DA
			} else {
				basedDA = coreda.NewDummyDA(100_000, 0, 0)
			}

			datastore, err := store.NewDefaultKVStore(nodeConfig.RootDir, nodeConfig.DBPath, "based")
			if err != nil {
				return fmt.Errorf("failed to create datastore: %w", err)
			}

			// Pass raw DA implementation and namespace to NewSequencer
			sequencer, err := based.NewSequencer(
				logger,
				basedDA,
				[]byte(nodeConfig.ChainID),
				basedStartHeight,
				basedMaxHeightDrift,
				datastore,
			)
			if err != nil {
				return fmt.Errorf("failed to create based sequencer: %w", err)
			}

			// Filter --evm and --based args without modifying os.Args
			filteredArgs := []string{os.Args[0]}
			for _, arg := range os.Args[1:] {
				if !((len(arg) > 6 && arg[:6] == "--evm.") || (len(arg) > 7 && arg[:7] == "--based.")) {
					filteredArgs = append(filteredArgs, arg)
				}
			}

			nodeKey, err := key.LoadNodeKey(filepath.Dir(nodeConfig.ConfigPath()))
			if err != nil {
				return fmt.Errorf("failed to load node key: %w", err)
			}

			p2pClient, err := p2p.NewClient(nodeConfig, nodeKey, datastore, logger, p2p.NopMetrics())
			if err != nil {
				return fmt.Errorf("failed to create P2P client: %w", err)
			}

			// Pass the raw rollDA implementation to StartNode.
			// StartNode might need adjustment if it strictly requires coreda.Client methods.
			// For now, assume it can work with coreda.DA or will be adjusted later.
			// We also need to pass the namespace config for rollDA.
			return rollcmd.StartNode(logger, cmd, executor, sequencer, rollDA, nodeKey, p2pClient, datastore, nodeConfig)
		},
	}

	rollconf.AddFlags(cmd)
	cmd.Flags().StringVar(&ethURL, "evm.eth-url", "http://localhost:8545", "Ethereum JSON-RPC URL")
	cmd.Flags().StringVar(&engineURL, "evm.engine-url", "http://localhost:8551", "Engine API URL")
	cmd.Flags().StringVar(&jwtSecret, "evm.jwt-secret", "", "JWT secret for Engine API")
	cmd.Flags().StringVar(&genesisHash, "evm.genesis-hash", "", "Genesis block hash")
	cmd.Flags().StringVar(&feeRecipient, "evm.fee-recipient", "", "Fee recipient address")
	cmd.Flags().StringVar(&basedURL, based.FlagBasedURL, "http://localhost:26658", "Based API URL")
	cmd.Flags().StringVar(&basedAuth, based.FlagBasedAuth, "", "Authentication token for Based API")
	cmd.Flags().StringVar(&basedNamespace, based.FlagBasedNamespace, "", "Namespace for Based API")
	cmd.Flags().Uint64Var(&basedStartHeight, based.FlagBasedStartHeight, 0, "Starting height for Based API")
	cmd.Flags().Uint64Var(&basedMaxHeightDrift, based.FlagBasedMaxHeightDrift, 1, "Maximum L1 block height drift")
	cmd.Flags().Float64Var(&basedGasMultiplier, based.FlagBasedGasMultiplier, 1.0, "Gas multiplier for Based API")
	cmd.Flags().Float64Var(&basedGasPrice, based.FlagBasedGasPrice, -1.0, "Gas price for Based API")

	return cmd
}
