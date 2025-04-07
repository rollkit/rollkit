package raft

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"

	// Consider using hclog for raft internal logs if needed
	// hclog "github.com/hashicorp/go-hclog"

	"github.com/rollkit/rollkit/attester/internal/config"
)

// NewRaftNode initializes and returns a new Raft node, transport, and potential error.
// It handles setting up configuration, transport, stores, and bootstrapping.
func NewRaftNode(cfg *config.Config, fsm raft.FSM, logger *slog.Logger) (*raft.Raft, raft.Transport, error) {
	raftLogger := logger.With("component", "raft")

	// --- Raft Configuration ---
	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(cfg.Node.ID)
	raftConfig.ElectionTimeout = cfg.Raft.GetElectionTimeout()   // Using getter for parsed duration
	raftConfig.HeartbeatTimeout = cfg.Raft.GetHeartbeatTimeout() // Using getter for parsed duration
	raftConfig.CommitTimeout = 100 * time.Millisecond            // Sensible default, adjust if needed
	raftConfig.SnapshotInterval = cfg.Raft.GetSnapshotInterval() // Using getter for parsed duration
	raftConfig.SnapshotThreshold = cfg.Raft.SnapshotThreshold

	// Configure Raft logger (optional, uses Discard by default)
	// raftConfig.Logger = hclog.New(&hclog.LoggerOptions{
	// 	Name:  "raft",
	// 	Level: hclog.LevelFromString("DEBUG"), // Or based on app log level
	// })

	raftDataDir := cfg.Raft.DataDir
	if err := os.MkdirAll(raftDataDir, 0700); err != nil {
		return nil, nil, fmt.Errorf("failed to create raft data directory %s: %w", raftDataDir, err)
	}

	// --- Transport ---
	// Ensure the RaftBindAddress is correctly configured in NodeConfig
	if cfg.Node.RaftBindAddress == "" {
		return nil, nil, fmt.Errorf("node.raft_bind_address is required in configuration")
	}
	addr, err := net.ResolveTCPAddr("tcp", cfg.Node.RaftBindAddress)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve raft bind address %s: %w", cfg.Node.RaftBindAddress, err)
	}
	// TODO: Add options for TLS if needed
	// transport, err := raft.NewTCPTransport(...) with TLS options
	transport, err := raft.NewTCPTransport(
		cfg.Node.RaftBindAddress, // bindAddr
		addr,                     // advertiseAddr (can be the same or different)
		3,                        // maxPool (number of connections to maintain)
		10*time.Second,           // timeout
		os.Stderr,                // Use os.Stderr for transport logs, consistent with snapshot store
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create raft tcp transport on %s: %w", cfg.Node.RaftBindAddress, err)
	}
	raftLogger.Info("Raft transport initialized", "address", cfg.Node.RaftBindAddress)

	// --- Snapshot Store ---
	snapshotStore, err := raft.NewFileSnapshotStore(raftDataDir, 2, os.Stderr) // Keep 2 snapshots, log to stderr
	if err != nil {
		transport.Close() // Clean up transport on error
		return nil, nil, fmt.Errorf("failed to create raft file snapshot store in %s: %w", raftDataDir, err)
	}
	raftLogger.Info("Raft snapshot store initialized", "directory", raftDataDir)

	// --- Log and Stable Store (BoltDB) ---
	boltDBPath := filepath.Join(raftDataDir, "raft.db")
	// Options can be passed to BoltDB if needed
	logStore, err := raftboltdb.NewBoltStore(boltDBPath)
	if err != nil {
		transport.Close()
		return nil, nil, fmt.Errorf("failed to create bolt store (%s): %w", boltDBPath, err)
	}
	raftLogger.Info("Raft log/stable store initialized", "path", boltDBPath)
	// stableStore is implicitly handled by BoltStore

	// --- Instantiate Raft ---
	raftNode, err := raft.NewRaft(raftConfig, fsm, logStore, logStore, snapshotStore, transport)
	if err != nil {
		// Ensure stores and transport are closed on error
		logStore.Close()
		transport.Close()
		return nil, nil, fmt.Errorf("failed to instantiate raft node: %w", err)
	}
	raftLogger.Info("Raft node instantiated")

	// --- Bootstrapping ---
	if cfg.Raft.BootstrapCluster {
		raftLogger.Info("Attempting to bootstrap RAFT cluster as this node is configured to do so...")
		bootstrapConfig := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      raftConfig.LocalID,
					Address: transport.LocalAddr(),
				},
			},
		}
		// Bootstrap the cluster
		bootstrapFuture := raftNode.BootstrapCluster(bootstrapConfig)
		if err := bootstrapFuture.Error(); err != nil {
			// Check if the error is "already bootstrapped" which is acceptable
			if err == raft.ErrCantBootstrap {
				raftLogger.Info("Cluster already bootstrapped or has existing state. Proceeding.")
			} else {
				// Real bootstrap error
				raftNode.Shutdown() // Clean up raft node
				logStore.Close()
				transport.Close()
				return nil, nil, fmt.Errorf("failed to bootstrap raft cluster: %w", err)
			}
		} else {
			raftLogger.Info("Successfully bootstrapped RAFT cluster.")
		}
	} else {
		raftLogger.Info("Skipping bootstrap (bootstrap_cluster is false or joining existing cluster)")
		// Logic for joining an existing cluster via Peers config would go here if needed.
		// This often involves using raft.AddVoter() or ensuring the peer list is correct
		// for discovery by the transport. Hashicorp's docs have more details.
		if len(cfg.Raft.Peers) > 0 {
			raftLogger.Warn("raft.peers configuration is present but auto-joining logic is not implemented here.", "peers", cfg.Raft.Peers)
			// Example placeholder for manually adding peers if desired:
			// for _, peerAddr := range cfg.Raft.Peers {
			// 	 // Need a way to map peerAddr to server ID
			//   serverID := raft.ServerID("id_from_" + peerAddr)
			// 	 addFuture := raftNode.AddVoter(serverID, raft.ServerAddress(peerAddr), 0, 0)
			// 	 if err := addFuture.Error(); err != nil {
			// 		 raftLogger.Error("Failed to add peer as voter", "peer", peerAddr, "error", err)
			// 	 }
			// }
		}
	}

	// Return the concrete transport type which implements NetworkTransport
	return raftNode, transport, nil
}
