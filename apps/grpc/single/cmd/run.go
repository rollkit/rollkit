package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/evstack/ev-node/core/execution"
	"github.com/evstack/ev-node/da/jsonrpc"
	executiongrpc "github.com/evstack/ev-node/execution/grpc"
	"github.com/evstack/ev-node/node"
	rollcmd "github.com/evstack/ev-node/pkg/cmd"
	"github.com/evstack/ev-node/pkg/config"
	"github.com/evstack/ev-node/pkg/p2p"
	"github.com/evstack/ev-node/pkg/p2p/key"
	"github.com/evstack/ev-node/pkg/store"
	"github.com/evstack/ev-node/sequencers/single"
)

const (
	// FlagGrpcExecutorURL is the flag for the gRPC executor endpoint
	FlagGrpcExecutorURL = "grpc-executor-url"
)

var RunCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"node", "run"},
	Short:   "Run the rollkit node with gRPC execution client",
	Long: `Start a Rollkit node that connects to a remote execution client via gRPC.
The execution client must implement the Rollkit execution gRPC interface.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Create gRPC execution client
		executor, err := createGRPCExecutionClient(cmd)
		if err != nil {
			return err
		}

		// Parse node configuration
		nodeConfig, err := rollcmd.ParseConfig(cmd)
		if err != nil {
			return err
		}

		logger := rollcmd.SetupLogger(nodeConfig.Log)

		// Create DA client
		daJrpc, err := jsonrpc.NewClient(cmd.Context(), logger, nodeConfig.DA.Address, nodeConfig.DA.AuthToken, nodeConfig.DA.Namespace)
		if err != nil {
			return err
		}

		// Create datastore
		datastore, err := store.NewDefaultKVStore(nodeConfig.RootDir, nodeConfig.DBPath, "grpc-single")
		if err != nil {
			return err
		}

		// Create metrics provider
		singleMetrics, err := single.DefaultMetricsProvider(nodeConfig.Instrumentation.IsPrometheusEnabled())(nodeConfig.ChainID)
		if err != nil {
			return err
		}

		// Create sequencer
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

		// Load node key
		nodeKey, err := key.LoadNodeKey(filepath.Dir(nodeConfig.ConfigPath()))
		if err != nil {
			return err
		}

		// Create P2P client
		p2pClient, err := p2p.NewClient(nodeConfig, nodeKey, datastore, logger, nil)
		if err != nil {
			return err
		}

		// Start the node
		return rollcmd.StartNode(logger, cmd, executor, sequencer, &daJrpc.DA, p2pClient, datastore, nodeConfig, node.NodeOptions{})
	},
}

func init() {
	// Add rollkit configuration flags
	config.AddFlags(RunCmd)

	// Add gRPC-specific flags
	addGRPCFlags(RunCmd)
}

// createGRPCExecutionClient creates a new gRPC execution client from command flags
func createGRPCExecutionClient(cmd *cobra.Command) (execution.Executor, error) {
	// Get the gRPC executor URL from flags
	executorURL, err := cmd.Flags().GetString(FlagGrpcExecutorURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get '%s' flag: %w", FlagGrpcExecutorURL, err)
	}

	if executorURL == "" {
		return nil, fmt.Errorf("%s flag is required", FlagGrpcExecutorURL)
	}

	// Create and return the gRPC client
	return executiongrpc.NewClient(executorURL), nil
}

// addGRPCFlags adds flags specific to the gRPC execution client
func addGRPCFlags(cmd *cobra.Command) {
	cmd.Flags().String(FlagGrpcExecutorURL, "http://localhost:50051", "URL of the gRPC execution service")
}
