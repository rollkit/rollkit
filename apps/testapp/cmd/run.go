package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ipfs/go-datastore"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	kvexecutor "github.com/evstack/ev-node/apps/testapp/kv"
	"github.com/evstack/ev-node/block"
	"github.com/evstack/ev-node/core/execution"
	coresequencer "github.com/evstack/ev-node/core/sequencer"
	"github.com/evstack/ev-node/da/celestia"
	"github.com/evstack/ev-node/node"
	"github.com/evstack/ev-node/pkg/cmd"
	"github.com/evstack/ev-node/pkg/config"
	blobrpc "github.com/evstack/ev-node/pkg/da/jsonrpc"
	da "github.com/evstack/ev-node/pkg/da/types"
	"github.com/evstack/ev-node/pkg/genesis"
	"github.com/evstack/ev-node/pkg/p2p/key"
	"github.com/evstack/ev-node/pkg/sequencers/based"
	"github.com/evstack/ev-node/pkg/sequencers/single"
	"github.com/evstack/ev-node/pkg/sequencers/solo"
	"github.com/evstack/ev-node/pkg/store"
)

const testDbName = "testapp"

var RunCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"node", "run"},
	Short:   "Run the testapp node",
	RunE: func(command *cobra.Command, args []string) error {
		nodeConfig, err := cmd.ParseStartConfig(command)
		if err != nil {
			return err
		}

		logger := cmd.SetupLogger(nodeConfig.Log)

		// Get KV endpoint flag
		kvEndpoint, _ := command.Flags().GetString(flagKVEndpoint)
		if kvEndpoint == "" {
			logger.Info().Msg("KV endpoint flag not set, using default from http_server")
		}

		// Create test implementations
		executor, err := kvexecutor.NewKVExecutor(nodeConfig.RootDir, nodeConfig.DBPath)
		if err != nil {
			return err
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		headerNamespace := da.NamespaceFromString(nodeConfig.DA.GetNamespace())
		dataNamespace := da.NamespaceFromString(nodeConfig.DA.GetDataNamespace())

		logger.Info().Str("headerNamespace", headerNamespace.HexString()).Str("dataNamespace", dataNamespace.HexString()).Msg("namespaces")

		nodeKey, err := key.LoadNodeKey(filepath.Dir(nodeConfig.ConfigPath()))
		if err != nil {
			return err
		}

		datastore, err := store.NewDefaultKVStore(nodeConfig.RootDir, nodeConfig.DBPath, testDbName)
		if err != nil {
			return err
		}

		// Start the KV executor HTTP server
		if kvEndpoint != "" { // Only start if endpoint is provided
			httpServer := kvexecutor.NewHTTPServer(executor, kvEndpoint)
			err = httpServer.Start(ctx) // Use the main context for lifecycle management
			if err != nil {
				return fmt.Errorf("failed to start KV executor HTTP server: %w", err)
			} else {
				logger.Info().Str("endpoint", kvEndpoint).Msg("KV executor HTTP server started")
			}
		}

		genesisPath := filepath.Join(filepath.Dir(nodeConfig.ConfigPath()), "genesis.json")
		genesis, err := genesis.LoadGenesis(genesisPath)
		if err != nil {
			return fmt.Errorf("failed to load genesis: %w", err)
		}

		if genesis.DAStartHeight == 0 && !nodeConfig.Node.Aggregator {
			logger.Warn().Msg("da_start_height is not set in genesis.json, ask your chain developer")
		}

		// Create sequencer based on configuration
		sequencer, err := createSequencer(ctx, command, logger, datastore, nodeConfig, genesis, executor)
		if err != nil {
			return err
		}

		var daOpts []block.DAClientOption
		if nodeConfig.DA.TxSigner.Enabled && (nodeConfig.Node.Aggregator || nodeConfig.Node.Promotable) {
			signerCfg, err := celestia.ConfigFromNode(nodeConfig)
			if err != nil {
				return err
			}
			submitter, err := celestia.New(ctx, signerCfg, logger)
			if err != nil {
				return fmt.Errorf("failed to create DA tx signer: %w", err)
			}
			defer submitter.Close()
			daOpts = append(daOpts, block.WithSubmitter(submitter))
		}

		return cmd.StartNode(logger, command, executor, sequencer, nodeKey, datastore, nodeConfig, genesis, node.NodeOptions{}, daOpts...)
	},
}

// createSequencer creates a sequencer based on the configuration.
// If BasedSequencer is enabled, it creates a based sequencer that fetches transactions from DA.
// Otherwise, it creates a single (traditional) sequencer.
func createSequencer(
	ctx context.Context,
	cmd *cobra.Command,
	logger zerolog.Logger,
	datastore datastore.Batching,
	nodeConfig config.Config,
	genesis genesis.Genesis,
	executor execution.Executor,
) (coresequencer.Sequencer, error) {
	// Only aggregators and promotable nodes need a sequencer; promotable
	// followers must have one ready before they are promoted to proposer.
	if !nodeConfig.Node.Aggregator && !nodeConfig.Node.Promotable {
		return nil, nil
	}

	if enabled, _ := cmd.Flags().GetBool(flagSoloSequencer); enabled {
		if nodeConfig.Node.BasedSequencer {
			return nil, fmt.Errorf("solo sequencer cannot be used with based")
		}

		return solo.NewSoloSequencer(logger, []byte(genesis.ChainID), executor), nil
	}

	blobClient, err := blobrpc.NewWSClient(ctx, logger, nodeConfig.GetDAAddress(), nodeConfig.DA.AuthToken, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create blob client: %w", err)
	}

	daClient := block.NewDAClient(blobClient, nodeConfig, logger)

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
		1_000_000,
		genesis,
		executor,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create single sequencer: %w", err)
	}

	return sequencer, nil
}
