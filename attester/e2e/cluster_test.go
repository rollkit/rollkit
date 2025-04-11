package e2e

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	attesterv1 "github.com/rollkit/rollkit/attester/api/gen/attester/v1"
	attesterconfig "github.com/rollkit/rollkit/attester/internal/config"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v3"
)

const (
	numFollowers       = 3
	numNodes           = numFollowers + 1
	baseGRPCPort       = 13000
	baseRaftPort       = 12000
	binaryName         = "attesterd"
	leaderID           = "sequencer-0"
	attesterIDPrefix   = "attester-"
	logLevel           = "debug"
	grpcDialTimeout    = 5 * time.Second
	interactionTimeout = 10 * time.Second
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

// Cluster manages the set of nodes in the E2E test
type Cluster struct {
	nodes      map[string]*nodeInfo
	allNodes   []*nodeInfo
	keysDir    string
	configsDir string
	logsDir    string
	testDir    string
	binaryPath string
}

// NewCluster creates a new Cluster instance with initialized directories and paths.
func NewCluster(t *testing.T, testDir, keysDir, configsDir, logsDir, binaryPath string) *Cluster {
	t.Helper()
	return &Cluster{
		nodes:      make(map[string]*nodeInfo, numNodes),
		allNodes:   make([]*nodeInfo, 0, numNodes),
		keysDir:    keysDir,
		configsDir: configsDir,
		logsDir:    logsDir,
		testDir:    testDir,
		binaryPath: binaryPath,
	}
}

// AddNodes creates the leader and follower node configurations, generates keys,
// assigns addresses, and populates the Cluster's node list.
// It returns a map of attester public keys needed for the leader's config.
func (c *Cluster) AddNodes(t *testing.T, numFollowers int) map[string]string {
	t.Helper()
	attesterPubKeys := make(map[string]string)

	leaderNode := c.generateNode(t, leaderID, true)
	leaderNode.raftAddr = netAddr("127.0.0.1", baseRaftPort+0)
	leaderNode.grpcAddr = netAddr("127.0.0.1", baseGRPCPort+0)
	c.nodes[leaderID] = leaderNode
	c.allNodes = append(c.allNodes, leaderNode)

	for i := 0; i < numFollowers; i++ {
		nodeID := fmt.Sprintf("%s%d", attesterIDPrefix, i)
		nodeIndex := i + 1
		followerNode := c.generateNode(t, nodeID, false)
		followerNode.raftAddr = netAddr("127.0.0.1", baseRaftPort+nodeIndex)
		followerNode.grpcAddr = netAddr("127.0.0.1", baseGRPCPort+nodeIndex)
		c.nodes[nodeID] = followerNode
		c.allNodes = append(c.allNodes, followerNode)
		attesterPubKeys[nodeID] = followerNode.pubFile
	}

	t.Logf("Generated keys and node info for %d nodes", len(c.allNodes))
	return attesterPubKeys
}

// GenerateAndWriteConfigs creates the configuration for each node in the cluster
// based on the cluster state and writes it to the respective config file.
func (c *Cluster) GenerateAndWriteConfigs(t *testing.T, attesterPubKeys map[string]string) {
	t.Helper()
	t.Logf("Generating and writing configuration files...")

	leaderNode := c.nodes[leaderID]
	if leaderNode == nil {
		t.Fatalf("Leader node (%s) not found in cluster nodes map", leaderID)
	}
	leaderConfig := createNodeConfig(t, leaderNode, c.allNodes, attesterPubKeys)
	writeConfigFile(t, leaderNode.cfgFile, leaderConfig)

	for _, node := range c.allNodes {
		if !node.isLeader {
			followerConfig := createNodeConfig(t, node, c.allNodes, nil)
			writeConfigFile(t, node.cfgFile, followerConfig)
		}
	}
	t.Logf("Configuration files generated for %d nodes", len(c.allNodes))
}

// generateNode is an internal helper for AddNodes.
// It prepares files/dirs and generates keys for a single node.
func (c *Cluster) generateNode(t *testing.T, nodeID string, isLeader bool) *nodeInfo {
	t.Helper()

	privKeyPath := filepath.Join(c.keysDir, fmt.Sprintf("%s.key", nodeID))
	pubKeyPath := filepath.Join(c.keysDir, fmt.Sprintf("%s.pub", nodeID))
	cfgFilePath := filepath.Join(c.configsDir, fmt.Sprintf("%s.yaml", nodeID))
	dataDirPath := filepath.Join(c.testDir, "data", nodeID)
	logFilePath := filepath.Join(c.logsDir, fmt.Sprintf("%s.log", nodeID))

	require.NoError(t, os.MkdirAll(dataDirPath, 0755), "Failed to create data dir for %s", nodeID)

	pubKeyBytes, privKeyBytes, err := ed25519.GenerateKey(nil)
	require.NoError(t, err, "Failed to generate key for %s", nodeID)
	require.NoError(t, os.WriteFile(privKeyPath, privKeyBytes, 0600), "Failed to write private key for %s", nodeID)
	require.NoError(t, os.WriteFile(pubKeyPath, pubKeyBytes, 0644), "Failed to write public key for %s", nodeID)

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
func createNodeConfig(t *testing.T, targetNode *nodeInfo, allClusterNodes []*nodeInfo, attesterPubKeys map[string]string) *attesterconfig.Config {
	t.Helper()

	peers := make([]attesterconfig.PeerConfig, 0, len(allClusterNodes)-1)
	for _, node := range allClusterNodes {
		if node.id != targetNode.id {
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
			ElectionTimeout:   1 * time.Second,
			HeartbeatTimeout:  500 * time.Millisecond,
			SnapshotInterval:  30 * time.Second,
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
			Timeout: 5 * time.Second,
		},
	}

	if targetNode.isLeader {
		cfg.GRPC.ListenAddress = targetNode.grpcAddr
		if len(attesterPubKeys) > 0 {
			cfg.Aggregator.QuorumThreshold = len(attesterPubKeys) + 1
		} else {
			cfg.Aggregator.QuorumThreshold = 1
			t.Logf("WARN: Leader %s configured with no attestors in Aggregator.Attesters map. Setting quorum to 1.", targetNode.id)
		}
		cfg.Aggregator.Attesters = attesterPubKeys
	} else {
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
		cfg.Network.SequencerSigEndpoint = leaderGrpcAddr
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

// netAddr constructs a simple network address
func netAddr(ip string, port int) string {
	return fmt.Sprintf("%s:%d", ip, port)
}

// launchNode starts a node process
func (c *Cluster) LaunchNode(t *testing.T, node *nodeInfo) {
	t.Helper()
	args := []string{
		"--config", node.cfgFile,
		"--log-level", logLevel,
	}
	if node.isLeader {
		args = append(args, "--leader")
	}

	cmd := exec.Command(c.binaryPath, args...)
	cmd.Stdout = node.logFile
	cmd.Stderr = node.logFile

	err := cmd.Start()
	require.NoError(t, err, "Failed to start node %s", node.id)
	node.cmd = cmd
	t.Logf("Launched node %s (PID: %d) with config %s, log %s", node.id, cmd.Process.Pid, node.cfgFile, node.logFile.Name())
}

// cleanupCluster stops nodes and removes temporary files/dirs
func (c *Cluster) Cleanup(t *testing.T) {
	t.Helper()
	t.Log("Cleaning up E2E cluster...")
	for _, node := range c.nodes {
		if node.cmd != nil && node.cmd.Process != nil {
			pid := node.cmd.Process.Pid
			t.Logf("Stopping node %s (PID: %d)...", node.id, pid)
			if err := node.cmd.Process.Signal(os.Interrupt); err != nil {
				t.Logf("WARN: Failed to send SIGTERM to %s (PID: %d): %v. Attempting Kill.", node.id, pid, err)
			}
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
	if c.testDir != "" {
		t.Logf("Removing test directory: %s", c.testDir)
		if !t.Failed() {
			require.NoError(t, os.RemoveAll(c.testDir), "Failed to remove test directory")
		} else {
			t.Logf("Test failed, leaving test directory for inspection: %s", c.testDir)
		}
	}
	t.Log("Cleanup complete.")
}

// LaunchAllNodes starts all the nodes defined in the cluster configuration.
func (c *Cluster) LaunchAllNodes(t *testing.T) {
	t.Helper()
	t.Logf("Launching all %d nodes...", len(c.allNodes))
	for _, node := range c.allNodes {
		c.LaunchNode(t, node)
		time.Sleep(100 * time.Millisecond)
	}
	t.Logf("All nodes launched.")
}

// GetLeaderNode finds and returns the designated leader node.
func (c *Cluster) GetLeaderNode(t *testing.T) *nodeInfo {
	t.Helper()
	leaderNode, ok := c.nodes[leaderID] // Use the leaderID constant
	if !ok || leaderNode == nil {
		t.Fatalf("Leader node with ID '%s' not found in cluster nodes map", leaderID)
	}
	if !leaderNode.isLeader {
		t.Fatalf("Node '%s' found but is not marked as leader", leaderID)
	}
	require.NotEmpty(t, leaderNode.grpcAddr, "Leader node '%s' found but has no gRPC address assigned", leaderID)
	return leaderNode
}

// WaitFor logs a message and then pauses execution for the specified duration.
func (c *Cluster) WaitFor(t *testing.T, d time.Duration, reason string) {
	t.Helper()
	t.Logf("Waiting %v: %s", d, reason)
	time.Sleep(d)
}

// createTestDirs creates the temporary directories needed for the test.
func createTestDirs(t *testing.T) (testDir, keysDir, configsDir, logsDir, buildDir string) {
	t.Helper()
	baseDir, err := os.MkdirTemp("", "attester_e2e_test_*")
	require.NoError(t, err, "Failed to create base temp dir")
	t.Logf("Created base test directory: %s", baseDir)

	testDir = baseDir
	keysDir = filepath.Join(baseDir, "keys")
	configsDir = filepath.Join(baseDir, "configs")
	logsDir = filepath.Join(baseDir, "logs")
	buildDir = filepath.Join(baseDir, "bin")

	require.NoError(t, os.Mkdir(keysDir, 0755))
	require.NoError(t, os.Mkdir(configsDir, 0755))
	require.NoError(t, os.Mkdir(logsDir, 0755))
	require.NoError(t, os.Mkdir(buildDir, 0755))

	return
}

// compileBinary compiles the attesterd binary in the specified build directory.
func compileBinary(t *testing.T, buildDir string, binName string) (compiledBinaryPath string) {
	t.Helper()
	sourcePath := "../cmd/attesterd" // Adjust if the source path is different
	compiledBinaryPath = filepath.Join(buildDir, binName)
	t.Logf("Compiling attesterd binary from %s to %s", sourcePath, compiledBinaryPath)
	cmd := exec.Command("go", "build", "-o", compiledBinaryPath, sourcePath)
	err := cmd.Run()
	require.NoError(t, err, "Failed to compile attesterd binary")
	t.Logf("Successfully compiled attesterd binary.")

	_, err = os.Stat(compiledBinaryPath)
	require.NoError(t, err, "Compiled attesterd binary not found at %s", compiledBinaryPath)
	return
}

// setupCluster prepares the environment for the E2E test
func setupCluster(t *testing.T) *Cluster {
	t.Helper()

	testDir, keysDir, configsDir, logsDir, buildDir := createTestDirs(t)

	compiledBinaryPath := compileBinary(t, buildDir, binaryName)

	cluster := NewCluster(t, testDir, keysDir, configsDir, logsDir, compiledBinaryPath)

	attesterPubKeys := cluster.AddNodes(t, numFollowers)

	cluster.GenerateAndWriteConfigs(t, attesterPubKeys)

	t.Logf("Cluster setup complete for %d nodes in %s", len(cluster.allNodes), testDir) // Using numNodes constant implicitly via NewCluster

	return cluster
}

// triggerBlockProposal connects to the leader and sends a SubmitBlock request
func triggerBlockProposal(t *testing.T, leaderGRPCAddr string, height uint64, data string) {
	t.Helper()
	t.Logf("Attempting to connect to leader gRPC at %s to trigger proposal for height %d", leaderGRPCAddr, height)

	ctx, cancel := context.WithTimeout(context.Background(), grpcDialTimeout)
	defer cancel()
	conn, err := grpc.DialContext(ctx, leaderGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	require.NoError(t, err, "Failed to dial leader gRPC server at %s", leaderGRPCAddr)
	defer conn.Close()

	client := attesterv1.NewAttesterServiceClient(conn)

	// Generate a dummy hash for the test block
	hashBytes := sha256.Sum256([]byte(data))
	req := &attesterv1.SubmitBlockRequest{
		BlockHeight: height,
		BlockHash:   hashBytes[:],
		DataToSign:  []byte(data),
	}

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

	ctx, cancel := context.WithTimeout(context.Background(), grpcDialTimeout)
	defer cancel()
	conn, err := grpc.DialContext(ctx, nodeGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	require.NoError(t, err, "Failed to dial node gRPC server at %s", nodeGRPCAddr)
	defer conn.Close()

	client := attesterv1.NewAttesterServiceClient(conn)

	req := &attesterv1.GetAggregatedSignatureRequest{
		BlockHeight: height,
	}

	callCtx, callCancel := context.WithTimeout(context.Background(), interactionTimeout)
	defer callCancel()
	resp, err := client.GetAggregatedSignature(callCtx, req)
	require.NoError(t, err, "GetAggregatedSignature RPC call failed")
	require.NotNil(t, resp, "Received nil response from GetAggregatedSignature")

	t.Logf("Received GetAggregatedSignature response for height %d: QuorumMet=%t, Signatures=%d", height, resp.QuorumMet, len(resp.Signatures))
	return resp
}

// TriggerBlockProposalOnLeader finds the leader node and triggers a block proposal via gRPC.
func (c *Cluster) TriggerBlockProposalOnLeader(t *testing.T, height uint64, data string) {
	t.Helper()
	leaderNode := c.GetLeaderNode(t)
	triggerBlockProposal(t, leaderNode.grpcAddr, height, data)
}

// GetAggregatedSignaturesFromLeader finds the leader node and retrieves aggregated signatures via gRPC.
func (c *Cluster) GetAggregatedSignaturesFromLeader(t *testing.T, height uint64) *attesterv1.GetAggregatedSignatureResponse {
	t.Helper()
	leaderNode := c.GetLeaderNode(t)
	return getAggregatedSignatures(t, leaderNode.grpcAddr, height)
}

// VerifyQuorumMet checks if the quorum flag is set and the signature count is as expected.
func (c *Cluster) VerifyQuorumMet(t *testing.T, resp *attesterv1.GetAggregatedSignatureResponse, height uint64) {
	t.Helper()
	require.True(t, resp.QuorumMet, "Quorum flag should be set for height %d", height)

	expectedSigCount := numFollowers + 1
	require.Len(t, resp.Signatures, expectedSigCount, "Expected %d aggregated signatures for height %d, got %d", expectedSigCount, height, len(resp.Signatures))
	t.Logf("Quorum check successful for height %d: QuorumMet=%t, Signatures=%d (Expected=%d)", height, resp.QuorumMet, len(resp.Signatures), expectedSigCount)
}

// VerifySignatures checks that each signature in the response corresponds to a unique expected public key
// and is cryptographically valid for the given signedData.
// Assumes VerifyQuorumMet has already passed.
func (c *Cluster) VerifySignatures(t *testing.T, resp *attesterv1.GetAggregatedSignatureResponse, height uint64, signedData []byte) {
	t.Helper()

	expectedSigCount := numFollowers + 1 // Leader + all followers
	require.Len(t, resp.Signatures, expectedSigCount, "Signature count mismatch in VerifySignatures for height %d (Expected %d, Got %d)", height, expectedSigCount, len(resp.Signatures))

	expectedPubKeys := make(map[string]ed25519.PublicKey)
	for _, node := range c.allNodes {
		require.NotEmpty(t, node.pubFile, "Public key file path is empty for node ID '%s'", node.id)
		pubKeyBytes, err := os.ReadFile(node.pubFile)
		require.NoError(t, err, "Failed to read public key file '%s' for node '%s'", node.pubFile, node.id)
		require.NotEmpty(t, pubKeyBytes, "Public key file '%s' for node '%s' is empty", node.pubFile, node.id)
		require.Len(t, pubKeyBytes, ed25519.PublicKeySize, "Public key size mismatch for node '%s'", node.id)
		expectedPubKeys[node.id] = ed25519.PublicKey(pubKeyBytes)
	}

	// Verify individual signatures against the set of expected keys
	t.Logf("Verifying %d individual signatures against %d expected public keys for height %d...", len(resp.Signatures), len(expectedPubKeys), height)
	verifiedSignaturesCount := 0
	usedKeys := make(map[string]bool) // Track which node IDs' keys have verified a signature

	for i, signature := range resp.Signatures {
		foundValidKey := false
		for nodeID, pubKey := range expectedPubKeys {
			if usedKeys[nodeID] { // Skip key if it already verified another signature
				continue
			}

			if ed25519.Verify(pubKey, signedData, signature) {
				t.Logf("Signature #%d verified successfully against public key of node '%s'", i+1, nodeID)
				usedKeys[nodeID] = true
				foundValidKey = true
				verifiedSignaturesCount++
				break
			}
		}
		if !foundValidKey {
			require.FailNow(t, fmt.Sprintf("Signature #%d could not be verified by any available expected public key for height %d", i+1, height))
		}
	}

	require.Equal(t, expectedSigCount, verifiedSignaturesCount, "Number of successfully verified signatures (%d) does not match expected count (%d) for height %d", verifiedSignaturesCount, expectedSigCount, height)

	t.Logf("All %d individual signatures verified successfully against unique expected public keys for height %d.", verifiedSignaturesCount, height)
}
