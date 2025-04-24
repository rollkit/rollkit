package cmd

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/rollkit/go-execution-evm"
	coreda "github.com/rollkit/rollkit/core/da"
	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/da/proxy/jsonrpc"
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

			basedURL, err = cmd.Flags().GetString("based.url")
			if err != nil {
				return fmt.Errorf("failed to get 'based.url' flag: %w", err)
			}
			basedAuth, err = cmd.Flags().GetString("based.auth")
			if err != nil {
				return fmt.Errorf("failed to get 'based.auth' flag: %w", err)
			}
			basedNamespace, err = cmd.Flags().GetString("based.namespace")
			if err != nil {
				return fmt.Errorf("failed to get 'based.namespace' flag: %w", err)
			}
			basedStartHeight, err = cmd.Flags().GetUint64("based.start-height")
			if err != nil {
				return fmt.Errorf("failed to get 'based.start-height' flag: %w", err)
			}
			basedMaxHeightDrift, err = cmd.Flags().GetUint64("based.max-height-drift")
			if err != nil {
				return fmt.Errorf("failed to get 'based.max-height-drift' flag: %w", err)
			}
			basedGasMultiplier, err = cmd.Flags().GetFloat64("based.gas-multiplier")
			if err != nil {
				return fmt.Errorf("failed to get 'based.gas-multiplier' flag: %w", err)
			}
			basedGasPrice, err = cmd.Flags().GetFloat64("based.gas-price")
			if err != nil {
				return fmt.Errorf("failed to get 'based.gas-price' flag: %w", err)
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
				client, err := jsonrpc.NewClient(ctx, logger, nodeConfig.DA.Address, nodeConfig.DA.AuthToken)
				if err != nil {
					return fmt.Errorf("failed to create DA client: %w", err)
				}
				rollDA = &client.DA
			} else {
				rollDA = coreda.NewDummyDA(100_000, 0, 0)
			}
			rollDALC := da.NewDAClient(rollDA, nodeConfig.DA.GasPrice, nodeConfig.DA.GasMultiplier, []byte(nodeConfig.DA.Namespace), nil, logger)

			var basedDA coreda.DA
			if basedAuth != "" {
				client, err := jsonrpc.NewClient(ctx, logger, basedURL, basedAuth)
				if err != nil {
					return fmt.Errorf("failed to create based client: %w", err)
				}
				basedDA = &client.DA
			} else {
				basedDA = coreda.NewDummyDA(100_000, 0, 0)
			}
			nsBytes, err := hex.DecodeString(basedNamespace)
			if err != nil {
				return fmt.Errorf("failed to decode based namespace: %w", err)
			}
			basedDALC := da.NewDAClient(basedDA, basedGasPrice, basedGasMultiplier, nsBytes, nil, logger)

			datastore, err := store.NewDefaultKVStore(nodeConfig.RootDir, nodeConfig.DBPath, "based")
			if err != nil {
				return fmt.Errorf("failed to create datastore: %w", err)
			}

			sequencer, err := based.NewSequencer(
				logger,
				basedDA,
				basedDALC,
				[]byte("rollkit-test"),
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

			return rollcmd.StartNode(logger, cmd, executor, sequencer, rollDALC, nodeKey, p2pClient, datastore, nodeConfig)
		},
	}

	rollconf.AddFlags(cmd)
	cmd.Flags().StringVar(&ethURL, "evm.eth-url", "http://localhost:8545", "Ethereum JSON-RPC URL")
	cmd.Flags().StringVar(&engineURL, "evm.engine-url", "http://localhost:8551", "Engine API URL")
	cmd.Flags().StringVar(&jwtSecret, "evm.jwt-secret", "", "JWT secret for Engine API")
	cmd.Flags().StringVar(&genesisHash, "evm.genesis-hash", "", "Genesis block hash")
	cmd.Flags().StringVar(&feeRecipient, "evm.fee-recipient", "", "Fee recipient address")
	cmd.Flags().StringVar(&basedURL, "based.url", "http://localhost:26658", "Based API URL")
	cmd.Flags().StringVar(&basedAuth, "based.auth", "", "Authentication token for Based API")
	cmd.Flags().StringVar(&basedNamespace, "based.namespace", "", "Namespace for Based API")
	cmd.Flags().Uint64Var(&basedStartHeight, "based.start-height", 0, "Starting height for Based API")
	cmd.Flags().Uint64Var(&basedMaxHeightDrift, "based.max-height-drift", 1, "Maximum L1 block height drift")
	cmd.Flags().Float64Var(&basedGasMultiplier, "based.gas-multiplier", 1.0, "Gas multiplier for Based API")
	cmd.Flags().Float64Var(&basedGasPrice, "based.gas-price", -1.0, "Gas price for Based API")

	return cmd
}
