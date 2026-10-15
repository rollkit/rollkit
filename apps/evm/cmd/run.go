package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-datastore"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/evstack/ev-node/apps/evm/server"
	"github.com/evstack/ev-node/block"
	"github.com/evstack/ev-node/core/execution"
	coresequencer "github.com/evstack/ev-node/core/sequencer"
	"github.com/evstack/ev-node/da/celestia"
	"github.com/evstack/ev-node/execution/evm"
	"github.com/evstack/ev-node/node"
	rollcmd "github.com/evstack/ev-node/pkg/cmd"
	"github.com/evstack/ev-node/pkg/config"
	blobrpc "github.com/evstack/ev-node/pkg/da/jsonrpc"
	da "github.com/evstack/ev-node/pkg/da/types"
	genesispkg "github.com/evstack/ev-node/pkg/genesis"
	"github.com/evstack/ev-node/pkg/p2p/key"
	"github.com/evstack/ev-node/pkg/sequencers/based"
	"github.com/evstack/ev-node/pkg/sequencers/single"
	"github.com/evstack/ev-node/pkg/store"
)

const (
	flagForceInclusionServer = "force-inclusion-server"
	evmDbName                = "evm-single"
)

var RunCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"node", "run"},
	Short:   "Run the evolve node with EVM execution client",
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeConfig, err := rollcmd.ParseStartConfig(cmd)
		if err != nil {
			return err
		}

		logger := rollcmd.SetupLogger(nodeConfig.Log)

		// Create datastore first - needed by execution client for ExecMeta tracking
		datastore, err := store.NewDefaultKVStore(nodeConfig.RootDir, nodeConfig.DBPath, evmDbName)
		if err != nil {
			return err
		}

		tracingEnabled := nodeConfig.Instrumentation.IsTracingEnabled()
		executor, err := createExecutionClient(cmd, datastore, tracingEnabled, logger)
		if err != nil {
			return err
		}

		var daOpts []block.DAClientOption
		if nodeConfig.DA.TxSigner.Enabled && (nodeConfig.Node.Aggregator || nodeConfig.Node.Promotable) {
			signerCfg, err := celestia.ConfigFromNode(nodeConfig)
			if err != nil {
				return err
			}
			submitter, err := celestia.New(cmd.Context(), signerCfg, logger)
			if err != nil {
				return fmt.Errorf("failed to create DA tx signer: %w", err)
			}
			defer submitter.Close()
			daOpts = append(daOpts, block.WithSubmitter(submitter))
		}

		var daClient block.FullDAClient
		if nodeConfig.DAEnabled() {
			blobClient, err := blobrpc.NewWSClient(cmd.Context(), logger, nodeConfig.GetDAAddress(), nodeConfig.DA.AuthToken, "")
			if err != nil {
				return fmt.Errorf("failed to create blob client: %w", err)
			}
			defer blobClient.Close()
			daClient = block.NewDAClient(blobClient, nodeConfig, logger, daOpts...)
		} else {
			logger.Info().Msg("DA address is not configured; syncing through P2P only")
		}

		headerNamespace := da.NamespaceFromString(nodeConfig.DA.GetNamespace())
		dataNamespace := da.NamespaceFromString(nodeConfig.DA.GetDataNamespace())

		logger.Info().Str("headerNamespace", headerNamespace.HexString()).Str("dataNamespace", dataNamespace.HexString()).Msg("namespaces")

		genesisPath := filepath.Join(filepath.Dir(nodeConfig.ConfigPath()), "genesis.json")
		genesis, err := genesispkg.LoadGenesis(genesisPath)
		if err != nil {
			return fmt.Errorf("failed to load genesis: %w", err)
		}

		if genesis.DAStartHeight == 0 && !nodeConfig.Node.Aggregator {
			logger.Warn().Msg("da_start_height is not set in genesis.json, ask your chain developer")
		}

		// Create sequencer based on configuration
		sequencer, err := createSequencer(cmd.Context(), logger, datastore, nodeConfig, genesis, daClient, executor)
		if err != nil {
			return err
		}

		nodeKey, err := key.LoadNodeKey(filepath.Dir(nodeConfig.ConfigPath()))
		if err != nil {
			return err
		}

		// Start force inclusion API server if address is provided
		forceInclusionAddr, err := cmd.Flags().GetString(flagForceInclusionServer)
		if err != nil {
			return fmt.Errorf("failed to get '%s' flag: %w", flagForceInclusionServer, err)
		}

		if forceInclusionAddr != "" {
			if daClient == nil {
				return fmt.Errorf("force inclusion server requires a DA address")
			}

			ethURL, err := cmd.Flags().GetString(evm.FlagEvmEthURL)
			if err != nil {
				return fmt.Errorf("failed to get '%s' flag: %w", evm.FlagEvmEthURL, err)
			}

			forceInclusionServer, err := server.NewForceInclusionServer(
				forceInclusionAddr,
				daClient,
				nodeConfig,
				genesis,
				logger,
				ethURL,
			)
			if err != nil {
				return fmt.Errorf("failed to create force inclusion server: %w", err)
			}

			if err := forceInclusionServer.Start(cmd.Context()); err != nil {
				return fmt.Errorf("failed to start force inclusion API server: %w", err)
			}

			// Ensure server is stopped when node stops
			defer func() {
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := forceInclusionServer.Stop(shutdownCtx); err != nil {
					logger.Error().Err(err).Msg("failed to stop force inclusion API server")
				}
			}()
		}

		return rollcmd.StartNode(logger, cmd, executor, sequencer, nodeKey, datastore, nodeConfig, genesis, node.NodeOptions{}, daOpts...)
	},
}

func init() {
	config.AddFlags(RunCmd)
	addFlags(RunCmd)
}

// createSequencer creates a sequencer based on the configuration.
// If BasedSequencer is enabled, it creates a based sequencer that fetches transactions from DA.
// Otherwise, it creates a single (traditional) sequencer.
func createSequencer(
	ctx context.Context,
	logger zerolog.Logger,
	datastore datastore.Batching,
	nodeConfig config.Config,
	genesis genesispkg.Genesis,
	daClient block.FullDAClient,
	executor execution.Executor,
) (coresequencer.Sequencer, error) {
	// Only aggregators and promotable nodes need a sequencer; promotable
	// followers must have one ready before they are promoted to proposer.
	if !nodeConfig.Node.Aggregator && !nodeConfig.Node.Promotable {
		return nil, nil
	}

	if nodeConfig.Node.BasedSequencer {
		// Based sequencer mode - fetch transactions only from DA
		if !nodeConfig.Node.Aggregator {
			return nil, fmt.Errorf("based sequencer mode requires aggregator mode to be enabled")
		}

		basedSeq, err := based.NewBasedSequencer(ctx, daClient, nodeConfig, datastore, genesis, logger, executor)
		if err != nil {
			return nil, fmt.Errorf("failed to create based sequencer: %w", err)
		}

		logger.Info().
			Str("forced_inclusion_namespace", nodeConfig.DA.GetForcedInclusionNamespace()).
			Uint64("da_epoch", genesis.DAEpochForcedInclusion).
			Msg("based sequencer initialized")

		return basedSeq, nil
	}

	sequencer, err := single.NewSequencer(
		logger,
		datastore,
		daClient,
		nodeConfig,
		[]byte(genesis.ChainID),
		1000,
		genesis,
		executor,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create single sequencer: %w", err)
	}

	return sequencer, nil
}

func createExecutionClient(cmd *cobra.Command, db datastore.Batching, tracingEnabled bool, logger zerolog.Logger) (execution.Executor, error) {
	// Read execution client parameters from flags
	ethURL, err := cmd.Flags().GetString(evm.FlagEvmEthURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get '%s' flag: %w", evm.FlagEvmEthURL, err)
	}
	engineURL, err := cmd.Flags().GetString(evm.FlagEvmEngineURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get '%s' flag: %w", evm.FlagEvmEngineURL, err)
	}

	// Get JWT secret file path
	jwtSecretFile, err := cmd.Flags().GetString(evm.FlagEvmJWTSecretFile)
	if err != nil {
		return nil, fmt.Errorf("failed to get '%s' flag: %w", evm.FlagEvmJWTSecretFile, err)
	}

	if jwtSecretFile == "" {
		return nil, fmt.Errorf("JWT secret file must be provided via --evm.jwt-secret-file")
	}

	// Read JWT secret from file
	secretBytes, err := os.ReadFile(jwtSecretFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read JWT secret from file '%s': %w", jwtSecretFile, err)
	}
	jwtSecret := string(bytes.TrimSpace(secretBytes))

	if jwtSecret == "" {
		return nil, fmt.Errorf("JWT secret file '%s' is empty", jwtSecretFile)
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

	return evm.NewEngineExecutionClient(ethURL, engineURL, jwtSecret, genesisHash, feeRecipient, db, tracingEnabled, logger)
}

// addFlags adds flags related to the EVM execution client
func addFlags(cmd *cobra.Command) {
	cmd.Flags().String(evm.FlagEvmEthURL, "http://localhost:8545", "URL of the Ethereum JSON-RPC endpoint")
	cmd.Flags().String(evm.FlagEvmEngineURL, "http://localhost:8551", "URL of the Engine API endpoint")
	cmd.Flags().String(evm.FlagEvmJWTSecretFile, "", "Path to file containing the JWT secret for authentication")
	cmd.Flags().String(evm.FlagEvmGenesisHash, "", "Hash of the genesis block")
	cmd.Flags().String(evm.FlagEvmFeeRecipient, "", "Address that will receive transaction fees")
	cmd.Flags().String(flagForceInclusionServer, "", "Address for force inclusion API server (e.g. 127.0.0.1:8547). If set, enables the server for direct DA submission")
}
