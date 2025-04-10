package e2e

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v3"

	// Assuming config structure is accessible, adjust import path if needed
	attesterv1 "github.com/rollkit/rollkit/attester/api/gen/attester/v1"
	attesterconfig "github.com/rollkit/rollkit/attester/internal/config"
)

const (
	numFollowers       = 3
	numNodes           = numFollowers + 1 // 1 Leader + Followers
	baseGRPCPort       = 13000
	baseRaftPort       = 12000
	defaultWaitTime    = 5 * time.Second
	interactionTimeout = 10 * time.Second
	testBlockHeight    = 100
	testBlockDataStr   = "test-block-data-e2e"
	buildDir           = "../bin"
	binaryName         = "attesterd"
	binaryPath         = buildDir + "/" + binaryName
	leaderID           = "sequencer-0"
	attesterIDPrefix   = "attester-"
	logLevel           = "debug"
	grpcDialTimeout    = 5 * time.Second
)

// nodeInfo holds runtime information about a launched node process
type nodeInfo struct {
	id       string
	cmd      *exec.Cmd
	cfgFile  string
	keyFile  string
	pubFile  string
	dataDir  string
	isLeader bool
	logFile  *os.File
	grpcAddr string
	raftAddr string
}

// clusterInfo manages the set of nodes in the E2E test
type clusterInfo struct {
	nodes      map[string]*nodeInfo // Map nodeID -> nodeInfo
	allNodes   []*nodeInfo          // Ordered list might be useful too
	keysDir    string               // Directory for generated keys
	configsDir string               // Directory for generated configs
	logsDir    string               // Directory for node logs
	testDir    string               // Base temporary directory for the test
	binaryPath string
}

// setupCluster prepares the environment for the E2E test
func setupCluster(t *testing.T) *clusterInfo {
	t.Helper()

	// 1. Create base temporary directory for this test run
	testDir, err := os.MkdirTemp("", "attester_e2e_test_*")
	require.NoError(t, err, "Failed to create base temp dir")
	t.Logf("Created base test directory: %s", testDir)

	// Create subdirectories
	keysDir := filepath.Join(testDir, "keys")
	configsDir := filepath.Join(testDir, "configs")
	logsDir := filepath.Join(testDir, "logs")
	buildDir := filepath.Join(testDir, "bin") // Directory for the compiled binary
	require.NoError(t, os.Mkdir(keysDir, 0755))
	require.NoError(t, os.Mkdir(configsDir, 0755))
	require.NoError(t, os.Mkdir(logsDir, 0755))
	require.NoError(t, os.Mkdir(buildDir, 0755)) // Create build directory

	// 2. Compile the attesterd binary
	sourcePath := "../cmd/attesterd" // Adjust if the source path is different
	compiledBinaryPath := filepath.Join(buildDir, binaryName)
	t.Logf("Compiling attesterd binary from %s to %s", sourcePath, compiledBinaryPath)
	cmd := exec.Command("go", "build", "-o", compiledBinaryPath, sourcePath)
	cmd.Stdout = os.Stdout // Optional: show build output
	cmd.Stderr = os.Stderr // Optional: show build errors
	err = cmd.Run()
	require.NoError(t, err, "Failed to compile attesterd binary")
	t.Logf("Successfully compiled attesterd binary.")

	// Ensure the compiled binary exists
	_, err = os.Stat(compiledBinaryPath)
	require.NoError(t, err, "Compiled attesterd binary not found at %s", compiledBinaryPath)

	cluster := &clusterInfo{
		nodes:      make(map[string]*nodeInfo, numNodes),
		allNodes:   make([]*nodeInfo, 0, numNodes), // Initialize slice
		keysDir:    keysDir,
		configsDir: configsDir,
		logsDir:    logsDir,
		testDir:    testDir,
		binaryPath: compiledBinaryPath, // Use the newly compiled binary path
	}

	// 3. Generate Keys and Configs for all nodes
	attesterPubKeys := make(map[string]string) // For leader's aggregator config

	// Generate leader info first
	leaderNode := generateNodeConfigAndKey(t, cluster, leaderID, 0, true)
	leaderNode.raftAddr = netAddr("127.0.0.1", baseRaftPort+0)
	leaderNode.grpcAddr = netAddr("127.0.0.1", baseGRPCPort+0)
	cluster.nodes[leaderID] = leaderNode
	cluster.allNodes = append(cluster.allNodes, leaderNode) // Add to list

	// Generate follower info
	for i := 0; i < numFollowers; i++ {
		nodeID := fmt.Sprintf("%s%d", attesterIDPrefix, i)
		nodeIndex := i + 1 // 0 is leader
		followerNode := generateNodeConfigAndKey(t, cluster, nodeID, nodeIndex, false)
		followerNode.raftAddr = netAddr("127.0.0.1", baseRaftPort+nodeIndex)
		cluster.nodes[nodeID] = followerNode
		cluster.allNodes = append(cluster.allNodes, followerNode) // Add to list
		attesterPubKeys[nodeID] = followerNode.pubFile
	}

	// 4. Write config files (now that all peers and keys are known)
	// Pass the full cluster.allNodes list to createNodeConfig
	leaderConfig := createNodeConfig(t, cluster.nodes[leaderID], cluster.allNodes, attesterPubKeys)
	writeConfigFile(t, cluster.nodes[leaderID].cfgFile, leaderConfig)

	for i := 0; i < numFollowers; i++ {
		nodeID := fmt.Sprintf("%s%d", attesterIDPrefix, i)
		followerNode := cluster.nodes[nodeID]
		// Pass the full cluster.allNodes list to createNodeConfig
		followerConfig := createNodeConfig(t, followerNode, cluster.allNodes, nil) // Followers don't need attester pubkeys
		writeConfigFile(t, followerNode.cfgFile, followerConfig)
	}

	t.Logf("Generated keys and configs for %d nodes in %s", numNodes, testDir)
	return cluster
}

// generateNodeConfigAndKey prepares files/dirs for a single node
func generateNodeConfigAndKey(t *testing.T, cluster *clusterInfo, nodeID string, nodeIndex int, isLeader bool) *nodeInfo {
	t.Helper()
	// Paths
	privKeyPath := filepath.Join(cluster.keysDir, fmt.Sprintf("%s.key", nodeID))
	pubKeyPath := filepath.Join(cluster.keysDir, fmt.Sprintf("%s.pub", nodeID))
	cfgFilePath := filepath.Join(cluster.configsDir, fmt.Sprintf("%s.yaml", nodeID))
	dataDirPath := filepath.Join(cluster.testDir, "data", nodeID)
	logFilePath := filepath.Join(cluster.logsDir, fmt.Sprintf("%s.log", nodeID))

	// Create data dir
	require.NoError(t, os.MkdirAll(dataDirPath, 0755), "Failed to create data dir for %s", nodeID)

	// Generate Keys
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err, "Failed to generate key for %s", nodeID)
	require.NoError(t, os.WriteFile(privKeyPath, privKey, 0600), "Failed to write private key for %s", nodeID)
	require.NoError(t, os.WriteFile(pubKeyPath, pubKey, 0644), "Failed to write public key for %s", nodeID)

	// Create log file
	logFile, err := os.Create(logFilePath)
	require.NoError(t, err, "Failed to create log file for %s", nodeID)

	return &nodeInfo{
		id:       nodeID,
		cfgFile:  cfgFilePath,
		keyFile:  privKeyPath,
		pubFile:  pubKeyPath,
		dataDir:  dataDirPath,
		isLeader: isLeader,
		logFile:  logFile,
	}
}

// createNodeConfig creates the config structure for a node
// Takes the target node and the list of all nodes in the cluster
func createNodeConfig(t *testing.T, targetNode *nodeInfo, allClusterNodes []*nodeInfo, attesterPubKeys map[string]string) *attesterconfig.Config {
	t.Helper()

	// Build the list of peers (excluding the target node itself)
	peers := make([]attesterconfig.PeerConfig, 0, len(allClusterNodes)-1)
	for _, node := range allClusterNodes {
		if node.id != targetNode.id { // Exclude self
			peers = append(peers, attesterconfig.PeerConfig{
				ID:      node.id,
				Address: node.raftAddr,
			})
		}
	}

	cfg := attesterconfig.Config{
		Node: attesterconfig.NodeConfig{
			ID:              targetNode.id,
			RaftBindAddress: targetNode.raftAddr,
		},
		Raft: attesterconfig.RaftConfig{
			DataDir:           targetNode.dataDir,
			Peers:             peers,
			BootstrapCluster:  targetNode.isLeader,
			ElectionTimeout:   "1s",
			HeartbeatTimeout:  "500ms",
			SnapshotInterval:  "30s",
			SnapshotThreshold: 100,
		},
		Signing: attesterconfig.SigningConfig{
			PrivateKeyPath: targetNode.keyFile,
			Scheme:         "ed25519",
		},
		Network: attesterconfig.NetworkConfig{
			SequencerSigEndpoint: "",
		},
		GRPC: attesterconfig.GRPCConfig{
			ListenAddress: "",
		},
		Aggregator: attesterconfig.AggregatorConfig{
			QuorumThreshold: 0,
			Attesters:       nil,
		},
		Execution: attesterconfig.ExecutionConfig{
			Enabled: false,
			Type:    "noop",
			Timeout: "5s",
		},
	}

	// Role-specific settings
	if targetNode.isLeader {
		cfg.GRPC.ListenAddress = targetNode.grpcAddr
		// Quorum calculation should depend on the number of *attesters* specified in the config map, not necessarily numFollowers
		// Let's assume for now the aggregator map contains all intended followers.
		if len(attesterPubKeys) > 0 {
			cfg.Aggregator.QuorumThreshold = len(attesterPubKeys) + 1 // Require all configured attestors + leader = N+1
		} else {
			cfg.Aggregator.QuorumThreshold = 1 // Only leader if no attestors configured (adjust logic as needed)
			t.Logf("WARN: Leader %s configured with no attestors in Aggregator.Attesters map. Setting quorum to 1.", targetNode.id)
		}
		cfg.Aggregator.Attesters = attesterPubKeys
	} else {
		// Follower needs leader's GRPC address
		// Find the leader node to get its GRPC address
		var leaderGrpcAddr string
		for _, node := range allClusterNodes {
			if node.isLeader {
				leaderGrpcAddr = node.grpcAddr
				break
			}
		}
		if leaderGrpcAddr == "" {
			t.Fatalf("Could not find leader node to set SequencerSigEndpoint for follower %s", targetNode.id)
		}
		cfg.Network.SequencerSigEndpoint = leaderGrpcAddr // Use actual leader's gRPC address
	}

	return &cfg
}

// writeConfigFile marshals and writes the config to a file
func writeConfigFile(t *testing.T, path string, cfg *attesterconfig.Config) {
	t.Helper()
	data, err := yaml.Marshal(cfg)
	require.NoError(t, err, "Failed to marshal config for %s", path)
	require.NoError(t, os.WriteFile(path, data, 0644), "Failed to write config file %s", path)
}

// netAddr constructs a simple network address (adjust if using hostnames/docker)
func netAddr(ip string, port int) string {
	// For local testing, assume localhost resolves or use 127.0.0.1
	// In more complex setups (docker), this might need service discovery/hostnames
	return fmt.Sprintf("%s:%d", ip, port)
}

// launchNode starts a node process
func launchNode(t *testing.T, cluster *clusterInfo, node *nodeInfo) {
	t.Helper()
	args := []string{
		"--config", node.cfgFile,
		"--log-level", logLevel,
	}
	if node.isLeader {
		args = append(args, "--leader")
	}

	cmd := exec.Command(cluster.binaryPath, args...)
	cmd.Stdout = node.logFile
	cmd.Stderr = node.logFile // Capture both stdout and stderr

	err := cmd.Start()
	require.NoError(t, err, "Failed to start node %s", node.id)
	node.cmd = cmd
	t.Logf("Launched node %s (PID: %d) with config %s, log %s", node.id, cmd.Process.Pid, node.cfgFile, node.logFile.Name())
}

// cleanupCluster stops nodes and removes temporary files/dirs
func cleanupCluster(t *testing.T, cluster *clusterInfo) {
	t.Helper()
	t.Log("Cleaning up E2E cluster...")
	for _, node := range cluster.nodes {
		if node.cmd != nil && node.cmd.Process != nil {
			pid := node.cmd.Process.Pid
			t.Logf("Stopping node %s (PID: %d)...", node.id, pid)
			// Send SIGTERM first
			if err := node.cmd.Process.Signal(os.Interrupt); err != nil {
				// Log error but proceed to Kill if Signal fails
				t.Logf("WARN: Failed to send SIGTERM to %s (PID: %d): %v. Attempting Kill.", node.id, pid, err)
			}
			// Give some time for graceful shutdown
			done := make(chan error, 1)
			go func() {
				_, err := node.cmd.Process.Wait()
				done <- err
			}()

			select {
			case <-time.After(5 * time.Second):
				t.Logf("WARN: Node %s (PID: %d) did not exit after 5s. Killing process.", node.id, pid)
				if killErr := node.cmd.Process.Kill(); killErr != nil {
					t.Logf("ERROR: Failed to kill process %s (PID: %d): %v", node.id, pid, killErr)
				}
			case err := <-done:
				if err != nil {
					// Process exited with an error, log it
					t.Logf("INFO: Node %s (PID: %d) exited with error: %v", node.id, pid, err)
				} else {
					t.Logf("INFO: Node %s (PID: %d) exited gracefully.", node.id, pid)
				}
			}
		}
		if node.logFile != nil {
			node.logFile.Close()
		}
	}
	if cluster.testDir != "" {
		t.Logf("Removing test directory: %s", cluster.testDir)
		// It's useful to keep logs on failure, conditionally remove:
		if !t.Failed() {
			require.NoError(t, os.RemoveAll(cluster.testDir), "Failed to remove test directory")
		} else {
			t.Logf("Test failed, leaving test directory for inspection: %s", cluster.testDir)
		}
	}
	t.Log("Cleanup complete.")
}

// triggerBlockProposal connects to the leader and sends a SubmitBlock request
func triggerBlockProposal(t *testing.T, leaderGRPCAddr string, height uint64, data string) {
	t.Helper()
	t.Logf("Attempting to connect to leader gRPC at %s to trigger proposal for height %d", leaderGRPCAddr, height)

	// Set up a connection to the server.
	ctx, cancel := context.WithTimeout(context.Background(), grpcDialTimeout)
	defer cancel()
	_ = ctx // Explicitly ignore the context variable to satisfy the linter
	conn, err := grpc.NewClient(leaderGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	require.NoError(t, err, "Failed to dial leader gRPC server at %s", leaderGRPCAddr)
	defer conn.Close()

	client := attesterv1.NewAttesterServiceClient(conn)

	// Prepare request
	// Generate a dummy hash for the test block
	hashBytes := sha256.Sum256([]byte(data))
	req := &attesterv1.SubmitBlockRequest{
		BlockHeight: height,
		BlockHash:   hashBytes[:],
		DataToSign:  []byte(data),
	}

	// Call SubmitBlock
	callCtx, callCancel := context.WithTimeout(context.Background(), interactionTimeout)
	defer callCancel()
	resp, err := client.SubmitBlock(callCtx, req)
	require.NoError(t, err, "SubmitBlock RPC call failed")
	require.NotNil(t, resp, "Received nil response from SubmitBlock")
	require.True(t, resp.Accepted, "SubmitBlock request was not accepted by the leader: %s", resp.ErrorMessage)

	t.Logf("Successfully triggered block proposal for height %d via gRPC", height)
}

// getAggregatedSignatures connects to a node and calls the GetAggregatedSignature RPC.
func getAggregatedSignatures(t *testing.T, nodeGRPCAddr string, height uint64) *attesterv1.GetAggregatedSignatureResponse {
	t.Helper()
	t.Logf("Attempting to connect to node gRPC at %s to get aggregated signatures for height %d", nodeGRPCAddr, height)

	// Set up a connection to the server.
	ctx, cancel := context.WithTimeout(context.Background(), grpcDialTimeout)
	defer cancel()
	_ = ctx // Explicitly ignore the context variable to satisfy the linter
	conn, err := grpc.NewClient(nodeGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	require.NoError(t, err, "Failed to dial node gRPC server at %s", nodeGRPCAddr)
	defer conn.Close()

	client := attesterv1.NewAttesterServiceClient(conn)

	// Prepare request
	req := &attesterv1.GetAggregatedSignatureRequest{
		BlockHeight: height,
	}

	// Call GetAggregatedSignature
	callCtx, callCancel := context.WithTimeout(context.Background(), interactionTimeout)
	defer callCancel()
	resp, err := client.GetAggregatedSignature(callCtx, req)
	require.NoError(t, err, "GetAggregatedSignature RPC call failed")
	require.NotNil(t, resp, "Received nil response from GetAggregatedSignature")

	t.Logf("Received GetAggregatedSignature response for height %d: QuorumMet=%t, Signatures=%d", height, resp.QuorumMet, len(resp.Signatures))
	return resp
}

// TestE2E_BasicAttestation runs a basic E2E scenario
func TestE2E_BasicAttestation(t *testing.T) {
	lvl := new(slog.LevelVar)
	lvl.Set(slog.LevelDebug)
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})))

	cluster := setupCluster(t)
	// Ensure cleanup happens even if the test panics or fails early
	defer cleanupCluster(t, cluster)

	// Launch all nodes
	for _, node := range cluster.nodes {
		launchNode(t, cluster, node)
		// Brief pause between starting nodes, might help leader election stability
		time.Sleep(100 * time.Millisecond)
	}

	t.Logf("Launched all %d nodes. Waiting %v for network to stabilize...", numNodes, defaultWaitTime)
	time.Sleep(defaultWaitTime) // Wait for leader election and connections

	// Get leader node info
	leaderNode := cluster.nodes[leaderID]
	require.NotNil(t, leaderNode, "Leader node info not found")
	require.True(t, leaderNode.isLeader, "Designated leader node is not marked as leader")
	require.NotEmpty(t, leaderNode.grpcAddr, "Leader gRPC address is empty")

	// Trigger the block proposal on the leader
	triggerBlockProposal(t, leaderNode.grpcAddr, testBlockHeight, testBlockDataStr)

	// Wait a bit for propagation and aggregation
	t.Logf("Waiting %v for attestation to complete...", defaultWaitTime)
	time.Sleep(defaultWaitTime)

	// Get the aggregated signatures from the leader
	aggregatedSigsResp := getAggregatedSignatures(t, leaderNode.grpcAddr, testBlockHeight)

	// Verify quorum was met and signatures were received
	require.True(t, aggregatedSigsResp.QuorumMet, "Quorum should be met for height %d", testBlockHeight)

	// Determine expected number of signatures based on config
	// In this test setup, quorum is N+1 (leader + all followers)
	expectedSigCount := numFollowers + 1
	require.Len(t, aggregatedSigsResp.Signatures, expectedSigCount, "Expected %d aggregated signatures, got %d", expectedSigCount, len(aggregatedSigsResp.Signatures))

	// Optional: Add more checks, e.g., verify the signatures themselves if public keys are known

	t.Log("E2E Test Completed Successfully!")
}
