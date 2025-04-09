package main

import (
	"context"
	"fmt"
	"os"

	"cosmossdk.io/log"

	"github.com/ethereum/go-ethereum/common"
	"github.com/rollkit/go-execution-evm"
	coreda "github.com/rollkit/rollkit/core/da"
	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/da/proxy/jsonrpc"
	rollcmd "github.com/rollkit/rollkit/pkg/cmd"
	"github.com/rollkit/rollkit/pkg/config"
	rollconf "github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/p2p"
	"github.com/rollkit/rollkit/pkg/p2p/key"
	"github.com/rollkit/rollkit/pkg/store"
	"github.com/rollkit/rollkit/sequencers/based"
	"github.com/spf13/cobra"
)

func NewExtendedRunNodeCmd(ctx context.Context) *cobra.Command {
	var (
		homePath     string
		ethURL       string
		engineURL    string
		jwtSecret    string
		genesisHash  string
		feeRecipient string

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
			homePath, err = cmd.Flags().GetString(config.FlagRootDir)
			if err != nil {
				return fmt.Errorf("error reading home flag: %w", err)
			}

			// Could add validation here if needed
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

			// Based flags
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
			logger := log.NewLogger(os.Stdout)

			nodeConfig, err := rollcmd.ParseConfig(cmd, homePath)
			if err != nil {
				panic(err)
			}

			executor, err := execution.NewEngineExecutionClient(
				ethURL, engineURL, jwtSecret, common.HexToHash(genesisHash), common.HexToAddress(feeRecipient),
			)
			if err != nil {
				return fmt.Errorf("failed to create execution client: %w", err)
			}

			// _, err = rollcmd.ParseConfig(cmd)
			// if err != nil {
			// 	return fmt.Errorf("failed to parse config: %w", err)
			// }

			// Create DA client with dummy DA
			// TODO: replace with actual DA client
			dummyDA1 := coreda.NewDummyDA(100_000, 0, 0)

			var (
				basedDA   coreda.DA
				basedDALC coreda.Client
			)
			if basedAuth != "" {
				client, err := jsonrpc.NewClient(ctx, basedURL, basedAuth)
				if err != nil {
					return fmt.Errorf("failed to create based client: %w", err)
				}
				basedDA = &client.DA
			} else {
				basedDA = coreda.NewDummyDA(100_000, 0, 0)
			}
			basedDALC = da.NewDAClient(basedDA, basedGasPrice, basedGasMultiplier, []byte(basedNamespace), nil, logger)

			dac := da.NewDAClient(dummyDA1, 0, 1.0, []byte("test"), []byte(""), logger)

			sequencer, err := based.NewSequencer(
				logger,
				basedDA,
				basedDALC,
				[]byte("rollkit-test"),
				basedStartHeight,
				basedMaxHeightDrift,
			)
			if err != nil {
				return fmt.Errorf("failed to create based sequencer: %w", err)
			}

			// Remove --evm and --based args from os.Args
			filteredArgs := []string{os.Args[0]}
			for _, arg := range os.Args[1:] {
				if !((len(arg) > 6 && arg[:6] == "--evm.") || (len(arg) > 7 && arg[:7] == "--based.")) {
					filteredArgs = append(filteredArgs, arg)
				}
			}
			os.Args = filteredArgs

			nodeKey, err := key.LoadNodeKey(nodeConfig.ConfigDir)
			if err != nil {
				panic(err)
			}

			datastore, err := store.NewDefaultKVStore(nodeConfig.RootDir, nodeConfig.DBPath, "based")
			if err != nil {
				panic(err)
			}

			p2pClient, err := p2p.NewClient(nodeConfig, "based", nodeKey, datastore, logger, p2p.NopMetrics())
			if err != nil {
				panic(err)
			}

			return rollcmd.StartNode(logger, cmd, executor, sequencer, dac, nodeKey, p2pClient, datastore, nodeConfig)
		},
	}

	// 👇 Add custom flags
	rollconf.AddFlags(cmd)
	cmd.Flags().StringVar(&homePath, "home", "~/.rollkit/based", "Home directory for the rollkit based node")

	cmd.Flags().StringVar(&ethURL, "evm.eth-url", "http://localhost:8545", "Ethereum JSON-RPC URL")
	cmd.Flags().StringVar(&engineURL, "evm.engine-url", "http://localhost:8551", "Engine API URL")
	cmd.Flags().StringVar(&jwtSecret, "evm.jwt-secret", "", "JWT secret for Engine API")
	cmd.Flags().StringVar(&genesisHash, "evm.genesis-hash", "", "Genesis block hash")
	cmd.Flags().StringVar(&feeRecipient, "evm.fee-recipient", "", "Fee recipient address")

	// Add based flags
	cmd.Flags().StringVar(&basedURL, "based.url", "http://localhost:26658", "Based API URL")
	cmd.Flags().StringVar(&basedAuth, "based.auth", "", "Authentication token for Based API")
	cmd.Flags().StringVar(&basedNamespace, "based.namespace", "", "Namespace for Based API")
	cmd.Flags().Uint64Var(&basedStartHeight, "based.start-height", 0, "Starting height for Based API")
	cmd.Flags().Uint64Var(&basedMaxHeightDrift, "based.max-height-drift", 1, "Maximum number of L1 (DA layer) block heights a rollup batch is allowed to drift while collecting sequenced transactions")
	cmd.Flags().Float64Var(&basedGasMultiplier, "based.gas-multiplier", 1.0, "Gas multiplier for Based API")
	cmd.Flags().Float64Var(&basedGasPrice, "based.gas-price", 1.0, "Gas price for Based API")

	return cmd
}

const (
	// AppName is the name of the application, the name of the command, and the name of the home directory.
	AppName = "based"
)

// RootCmd is the root command for Rollkit
var RootCmd = &cobra.Command{
	Use:   AppName,
	Short: "The first sovereign rollup framework that allows you to launch a sovereign, customizable blockchain as easily as a smart contract.",
	Long: `
Rollkit is the first sovereign rollup framework that allows you to launch a sovereign, customizable blockchain as easily as a smart contract.
If the --home flag is not specified, the rollkit command will create a folder "~/.testapp" where it will store node keys, config, and data.
`,
}

func main() {
	// Initiate the root command
	rootCmd := RootCmd
	// Create context for the executor
	ctx := context.Background()
	// Add subcommands to the root command
	rootCmd.AddCommand(
		rollcmd.NewDocsGenCmd(rootCmd, AppName),
		NewExtendedRunNodeCmd(ctx),
		rollcmd.VersionCmd,
		rollcmd.InitCmd,
	)

	if err := rootCmd.Execute(); err != nil {
		// Print to stderr and exit with error
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
