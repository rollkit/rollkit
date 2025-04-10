package raft

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"

	"github.com/rollkit/rollkit/attester/internal/config"
)

const (
	defaultRetainSnapshotCount = 2
	defaultRaftTimeout         = 10 * time.Second
	defaultHeartbeatTimeout    = 1 * time.Second
	defaultElectionTimeout     = 2 * time.Second
)

// NewRaftNode initializes and returns a new Raft node, transport, and potential error.
// It handles setting up configuration, transport, stores, and bootstrapping.
func NewRaftNode(cfg *config.Config, fsm raft.FSM, logger *slog.Logger) (*raft.Raft, raft.Transport, error) {
	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(cfg.Node.ID)
	raftConfig.ElectionTimeout = parseDuration(cfg.Raft.ElectionTimeout, defaultElectionTimeout)
	raftConfig.HeartbeatTimeout = parseDuration(cfg.Raft.HeartbeatTimeout, defaultHeartbeatTimeout)
	raftConfig.CommitTimeout = 100 * time.Millisecond // Example value, adjust as needed
	raftConfig.SnapshotInterval = parseDuration(cfg.Raft.SnapshotInterval, defaultRaftTimeout)
	raftConfig.SnapshotThreshold = cfg.Raft.SnapshotThreshold

	raftHclogLevel := determineRaftLogLevel(logger)
	raftConfig.Logger = hclog.New(&hclog.LoggerOptions{
		Name:            "raft",
		Level:           hclog.LevelFromString(raftHclogLevel),
		Output:          os.Stderr,
		IncludeLocation: false,
	})
	logger.Debug("Configured Raft internal logger", "level", raftHclogLevel)

	raftDataDir := cfg.Raft.DataDir
	if err := os.MkdirAll(raftDataDir, 0700); err != nil {
		logger.Error("Failed to create Raft data directory", "path", raftDataDir, "error", err)
		return nil, nil, fmt.Errorf("failed to create raft data directory %s: %w", raftDataDir, err)
	}

	if cfg.Node.RaftBindAddress == "" {
		logger.Error("Raft bind address not configured", "field", "node.raft_bind_address")
		return nil, nil, fmt.Errorf("node.raft_bind_address is required in configuration")
	}
	addr, err := net.ResolveTCPAddr("tcp", cfg.Node.RaftBindAddress)
	if err != nil {
		logger.Error("Failed to resolve Raft bind address", "address", cfg.Node.RaftBindAddress, "error", err)
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
		logger.Error("Failed to create Raft TCP transport", "address", cfg.Node.RaftBindAddress, "error", err)
		return nil, nil, fmt.Errorf("failed to create raft tcp transport on %s: %w", cfg.Node.RaftBindAddress, err)
	}
	logger.Info("Raft transport initialized", "address", transport.LocalAddr())

	snapshots, err := raft.NewFileSnapshotStore(raftDataDir, defaultRetainSnapshotCount, os.Stderr)
	if err != nil {
		if cerr := transport.Close(); cerr != nil {
			logger.Error("Failed to close transport after snapshot store error", "error", cerr)
		}
		logger.Error("Failed to create Raft file snapshot store", "path", raftDataDir, "error", err)
		return nil, nil, fmt.Errorf("failed to create raft file snapshot store in %s: %w", raftDataDir, err)
	}
	logger.Info("Raft snapshot store initialized", "directory", raftDataDir)

	boltDBPath := filepath.Join(raftDataDir, "raft.db")
	logStore, err := raftboltdb.NewBoltStore(boltDBPath)
	if err != nil {
		if cerr := transport.Close(); cerr != nil {
			logger.Error("Failed to close transport after bolt store error", "error", cerr)
		}
		logger.Error("Failed to create Raft BoltDB store", "path", boltDBPath, "error", err)
		return nil, nil, fmt.Errorf("failed to create bolt store (%s): %w", boltDBPath, err)
	}
	logger.Info("Raft log/stable store initialized", "path", boltDBPath)

	r, err := raft.NewRaft(raftConfig, fsm, logStore, logStore, snapshots, transport)
	if err != nil {
		logger.Error("Failed to instantiate Raft node", "error", err)
		if lcerr := logStore.Close(); lcerr != nil {
			logger.Error("Failed to close log store after raft instantiation error", "error", lcerr)
		}
		if tcerr := transport.Close(); tcerr != nil {
			logger.Error("Failed to close transport after raft instantiation error", "error", tcerr)
		}
		return nil, nil, fmt.Errorf("failed to instantiate raft node: %w", err)
	}
	logger.Info("Raft node instantiated")

	if cfg.Raft.BootstrapCluster {
		logger.Info("Attempting to bootstrap RAFT cluster...", "node_id", cfg.Node.ID)

		servers := make([]raft.Server, 0, len(cfg.Raft.Peers)+1)

		servers = append(servers, raft.Server{
			ID:      raftConfig.LocalID,
			Address: transport.LocalAddr(),
		})

		for _, peer := range cfg.Raft.Peers {
			if peer.ID == "" || peer.Address == "" {
				logger.Warn("Skipping peer with empty ID or Address during bootstrap", "peer_id", peer.ID, "peer_address", peer.Address)
				continue
			}
			servers = append(servers, raft.Server{
				ID:      raft.ServerID(peer.ID),
				Address: raft.ServerAddress(peer.Address),
			})
		}

		serverInfo := make([]string, len(servers))
		for i, s := range servers {
			serverInfo[i] = fmt.Sprintf("{ID:%s Addr:%s}", s.ID, s.Address)
		}
		logger.Debug("Bootstrapping cluster with configuration", "servers", serverInfo)

		configuration := raft.Configuration{Servers: servers}
		bootstrapFuture := r.BootstrapCluster(configuration)

		if err := bootstrapFuture.Error(); err != nil {
			if err == raft.ErrCantBootstrap {
				logger.Info("Cluster already bootstrapped or has existing state. Proceeding.")
			} else {
				logger.Error("Failed to bootstrap RAFT cluster", "error", err)
				shutdownErr := r.Shutdown().Error()
				if shutdownErr != nil {
					logger.Error("Error during shutdown after bootstrap failure", "error", shutdownErr)
				}
				if lcerr := logStore.Close(); lcerr != nil {
					logger.Error("Failed to close log store after bootstrap failure", "error", lcerr)
				}
				if tcerr := transport.Close(); tcerr != nil {
					logger.Error("Failed to close transport after bootstrap failure", "error", tcerr)
				}
				return nil, nil, fmt.Errorf("failed to bootstrap cluster: %w", err)
			}
		} else {
			logger.Info("Successfully bootstrapped RAFT cluster.")
		}
	} else {
		logger.Info("Skipping bootstrap (bootstrap_cluster is false or joining existing cluster)")
	}

	return r, transport, nil
}

// parseDuration parses a duration string, returning default if parsing fails or string is empty.
func parseDuration(durationStr string, defaultVal time.Duration) time.Duration {
	if durationStr == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(durationStr)
	if err != nil {
		slog.Warn("Failed to parse duration string, using default", "duration_string", durationStr, "default", defaultVal, "error", err)
		return defaultVal
	}
	return d
}

// determineRaftLogLevel maps slog levels to hclog levels.
func determineRaftLogLevel(logger *slog.Logger) string {
	level := "INFO"
	if logger.Enabled(context.Background(), slog.LevelDebug) {
		level = "DEBUG"
	}
	return level
}
