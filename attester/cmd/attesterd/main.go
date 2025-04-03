package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/rollkit/rollkit/attester/internal/config"
)

var rootCmd = &cobra.Command{
	Use:   "attesterd",
	Short: "Attester node daemon",
	Long:  `A daemon process that participates in the RAFT consensus to attest blocks.`,
	Run:   runNode,
}

// Variables for flags
var (
	configFile string
	logLevel   string
)

func init() {
	// Assume config is relative to the binary directory or CWD.
	// We might want it in $HOME/.attesterd/config.yaml or similar.
	// For now, relative to where it's executed.
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "attester.yaml", "Path to the configuration file")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Logging level (e.g., debug, info, warn, error)")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error executing command: %v\n", err)
		os.Exit(1)
	}
}

func runNode(cmd *cobra.Command, args []string) {
	log.Println("Initializing attester node...")

	// 1. Load configuration
	log.Printf("Loading configuration from %s...\n", configFile)
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	// TODO: Validate loaded configuration

	// 2. Initialize logger (Use a more advanced logger like slog or zerolog)
	log.Println("Initializing logger...") // Placeholder
	// logger := ... setup logger based on cfg.LogLevel ...

	// 3. Load private key
	log.Println("Loading signing key...")
	// signer, err := signing.LoadSigner(cfg.Signing)
	// if err != nil {
	//  log.Fatalf("Failed to load signing key: %v", err)
	// }
	// log.Printf("Loaded signing key with scheme: %s", signer.Scheme())

	// 4. Instantiate FSM
	log.Println("Initializing FSM...")
	// attesterFSM := fsm.NewAttesterFSM(logger, signer, cfg.Raft.DataDir)

	// 5. Instantiate RAFT node
	log.Println("Initializing RAFT node...")
	// raftNode, transport, err := raft.NewRaftNode(cfg, attesterFSM, logger)
	// if err != nil {
	//  log.Fatalf("Failed to initialize RAFT node: %v", err)
	// }
	// defer transport.Close() // Ensure transport is closed

	// 6. Instantiate gRPC server
	log.Println("Initializing gRPC server...")
	// grpcServer := grpc.NewAttesterServer(raftNode, logger)

	// 7. Start gRPC server in a goroutine
	log.Println("Starting gRPC server...")
	// go func() {
	//     if err := grpcServer.Start(cfg.GRPC.ListenAddress); err != nil {
	//         log.Fatalf("Failed to start gRPC server: %v", err)
	//     }
	// }()

	// 8. Handle graceful shutdown
	log.Println("Attester node running. Press Ctrl+C to exit.")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan // Block until a signal is received

	log.Println("Shutting down...")

	// Shutdown logic for gRPC and Raft
	// if grpcServer != nil {
	//     grpcServer.Stop()
	// }
	// if raftNode != nil {
	//     if err := raftNode.Shutdown().Error(); err != nil {
	//         log.Printf("Error shutting down RAFT node: %v", err)
	//     }
	// }

	log.Println("Shutdown complete.")
}
