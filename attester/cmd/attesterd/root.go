package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/rollkit/rollkit/attester/internal/config"
	"github.com/rollkit/rollkit/attester/internal/node"
	"github.com/spf13/cobra"
)

// Constants for flag names and defaults remain here or could be moved to a config package/file
const (
	flagConfig   = "config"
	flagLogLevel = "log-level"
	flagLeader   = "leader"

	defaultConfigPath = "attester.yaml"
	defaultLogLevel   = "info"
)

var (
	configFile string
	logLevel   string
	isLeader   bool
)

func init() {
	rootCmd.PersistentFlags().StringVar(&configFile, flagConfig, defaultConfigPath, "Path to the configuration file")
	rootCmd.PersistentFlags().StringVar(&logLevel, flagLogLevel, defaultLogLevel, "Logging level (e.g., debug, info, warn, error)")
	rootCmd.PersistentFlags().BoolVar(&isLeader, flagLeader, false, "Run the node as the RAFT leader (sequencer)")
}

var rootCmd = &cobra.Command{
	Use:   "attesterd",
	Short: "Attester node daemon",
	Long:  `A daemon process that participates in the RAFT consensus to attest blocks.`,
	Run:   runNode,
}

func runNode(cmd *cobra.Command, args []string) {
	logger := setupLogger()

	logger.Info("Initializing attester node", "role", map[bool]string{true: "LEADER", false: "FOLLOWER"}[isLeader])

	logger.Info("Loading configuration", "path", configFile)
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		logger.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	attesterNode, err := node.NewNode(cfg, logger, isLeader)
	if err != nil {
		logger.Error("Failed to create node", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := attesterNode.Start(ctx); err != nil {
		logger.Error("Failed to start node", "error", err)
		os.Exit(1)
	}

	logger.Info("Attester node running. Press Ctrl+C to exit.")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan

	logger.Info("Shutting down...")
	attesterNode.Stop()
	logger.Info("Shutdown complete.")
}

func setupLogger() *slog.Logger {
	var levelVar slog.LevelVar

	if err := levelVar.UnmarshalText([]byte(logLevel)); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid log level '%s', defaulting to '%s'. Error: %v\n", logLevel, defaultLogLevel, err)
		if defaultErr := levelVar.UnmarshalText([]byte(defaultLogLevel)); defaultErr != nil {
			panic(fmt.Sprintf("Error setting default log level: %v", defaultErr))
		}
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: levelVar.Level()}))
	slog.SetDefault(logger)
	return logger
}
