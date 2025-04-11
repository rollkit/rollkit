package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a temporary YAML config file for testing
func createTempConfigFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	tmpfn := filepath.Join(dir, "test_config.yaml")
	err := os.WriteFile(tmpfn, []byte(content), 0666)
	require.NoError(t, err, "Failed to write temporary config file")
	return tmpfn
}

// Helper function to create a dummy file (e.g., for private key path)
func createDummyFile(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	tmpfn := filepath.Join(dir, name)
	err := os.WriteFile(tmpfn, []byte("dummy key data"), 0666)
	require.NoError(t, err, "Failed to write dummy file")
	return tmpfn
}

func TestLoadConfig_Success(t *testing.T) {
	dummyKeyPath := createDummyFile(t, "key.pem")

	validYaml := `
node:
  id: "test-node-1"
  raft_bind_address: "127.0.0.1:8001"

raft:
  data_dir: "/tmp/attester-data"
  peers:
    - id: "peer-2"
      address: "127.0.0.1:8002"
    - id: "peer-3"
      address: "127.0.0.1:8003"
  bootstrap_cluster: false
  election_timeout: "600ms"
  heartbeat_timeout: "60ms"
  snapshot_interval: "3m"
  snapshot_threshold: 1000

grpc:
  listen_address: "127.0.0.1:9001"

signing:
  private_key_path: "` + dummyKeyPath + `"
  scheme: "ed25519"
`
	tmpConfigFile := createTempConfigFile(t, validYaml)

	cfg, err := LoadConfig(tmpConfigFile)

	require.NoError(t, err, "LoadConfig should succeed for valid config")
	require.NotNil(t, cfg, "Config object should not be nil")

	// Assertions for loaded values
	assert.Equal(t, "test-node-1", cfg.Node.ID)
	assert.Equal(t, "127.0.0.1:8001", cfg.Node.RaftBindAddress)
	assert.Equal(t, "/tmp/attester-data", cfg.Raft.DataDir)
	expectedPeers := []PeerConfig{
		{ID: "peer-2", Address: "127.0.0.1:8002"},
		{ID: "peer-3", Address: "127.0.0.1:8003"},
	}
	assert.Equal(t, expectedPeers, cfg.Raft.Peers)
	assert.False(t, cfg.Raft.BootstrapCluster)
	assert.Equal(t, 600*time.Millisecond, cfg.Raft.ElectionTimeout) // Check parsed duration directly
	assert.Equal(t, 60*time.Millisecond, cfg.Raft.HeartbeatTimeout) // Check parsed duration directly
	assert.Equal(t, 3*time.Minute, cfg.Raft.SnapshotInterval)       // Check parsed duration directly
	assert.Equal(t, uint64(1000), cfg.Raft.SnapshotThreshold)
	assert.Equal(t, "127.0.0.1:9001", cfg.GRPC.ListenAddress)
	assert.Equal(t, dummyKeyPath, cfg.Signing.PrivateKeyPath)
	assert.Equal(t, "ed25519", cfg.Signing.Scheme)
}

func TestLoadConfig_DefaultsApplied(t *testing.T) {
	dummyKeyPath := createDummyFile(t, "key.pem")
	minimalYaml := `
node:
  id: "minimal-node"
  raft_bind_address: "127.0.0.1:8005" # Required but not defaulted

raft:
  data_dir: "/tmp/minimal-data" # Required but not defaulted

signing:
  private_key_path: "` + dummyKeyPath + `" # Required but not defaulted
# scheme defaults to ed25519

grpc:
  listen_address: "127.0.0.1:9005" # Not defaulted, provided for completeness

# Network, Aggregator, Execution are omitted to test defaults
`
	tmpConfigFile := createTempConfigFile(t, minimalYaml)

	cfg, err := LoadConfig(tmpConfigFile)
	require.NoError(t, err, "LoadConfig should succeed with minimal config")
	require.NotNil(t, cfg)

	// Assert required fields are loaded
	assert.Equal(t, "minimal-node", cfg.Node.ID)
	assert.Equal(t, "/tmp/minimal-data", cfg.Raft.DataDir)
	assert.Equal(t, dummyKeyPath, cfg.Signing.PrivateKeyPath)

	// Assert default values are applied
	assert.Equal(t, 1*time.Second, cfg.Raft.ElectionTimeout, "Default Raft ElectionTimeout")
	assert.Equal(t, 500*time.Millisecond, cfg.Raft.HeartbeatTimeout, "Default Raft HeartbeatTimeout")
	assert.Equal(t, 120*time.Second, cfg.Raft.SnapshotInterval, "Default Raft SnapshotInterval")
	assert.Equal(t, uint64(8192), cfg.Raft.SnapshotThreshold, "Default Raft SnapshotThreshold")
	assert.Equal(t, "ed25519", cfg.Signing.Scheme, "Default Signing Scheme")
	assert.False(t, cfg.Execution.Enabled, "Default Execution Enabled")
	assert.Equal(t, "noop", cfg.Execution.Type, "Default Execution Type")
	assert.Equal(t, 15*time.Second, cfg.Execution.Timeout, "Default Execution Timeout")

	// Assert fields that are zero by default in Go if not set and have no viper default
	assert.Empty(t, cfg.Network.SequencerSigEndpoint, "Default Network SequencerSigEndpoint")
	assert.Zero(t, cfg.Aggregator.QuorumThreshold, "Default Aggregator QuorumThreshold")
	assert.Empty(t, cfg.Aggregator.Attesters, "Default Aggregator Attesters")
	assert.False(t, cfg.Raft.BootstrapCluster, "Default Raft BootstrapCluster") // Check zero value for bool
	assert.Empty(t, cfg.Raft.Peers, "Default Raft Peers")                       // Check zero value for slice
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("nonexistent/path/to/config.yaml")
	require.Error(t, err, "LoadConfig should fail if config file doesn't exist")
}

func TestLoadConfig_ValidationErrors(t *testing.T) {
	dummyKeyPath := createDummyFile(t, "key.pem")
	// Create a dummy file to test the PrivateKeyNotFound case
	invalidKeyPath := "/path/to/nonexistent/key.pem"

	tests := []struct {
		name                string
		yamlContent         string
		expectErrorContains string
	}{
		{
			name: "Invalid YAML Syntax",
			yamlContent: `
node:
  id: "bad-yaml"
 raft_bind_address: "-" # Bad indentation
`,
			expectErrorContains: "failed to read config file", // Viper error on read
		},
		{
			name: "Missing Required Field - Node ID",
			yamlContent: `
node:
  raft_bind_address: "127.0.0.1:8001"
raft:
  data_dir: "/tmp/data"
signing:
  private_key_path: "` + dummyKeyPath + `"
`,
			expectErrorContains: "node.id is required",
		},
		{
			name: "Missing Required Field - Signing Key Path",
			yamlContent: `
node:
  id: "node1"
  raft_bind_address: "127.0.0.1:8001"
raft:
  data_dir: "/tmp/data"
signing:
  scheme: "ed25519"
`,
			expectErrorContains: "signing.private_key_path is required",
		},
		{
			name: "Missing Required Field - Raft Data Dir",
			yamlContent: `
node:
  id: "node1"
  raft_bind_address: "127.0.0.1:8001"
signing:
  private_key_path: "` + dummyKeyPath + `"
`,
			expectErrorContains: "raft.data_dir is required",
		},
		{
			name: "Invalid Duration Format",
			yamlContent: `
node:
  id: "node1"
  raft_bind_address: "127.0.0.1:8001"
raft:
  data_dir: "/tmp/data"
  election_timeout: "not-a-duration" 
signing:
  private_key_path: "` + dummyKeyPath + `"
`,
			expectErrorContains: "failed to unmarshal config", // mapstructure error on duration parse
		},
		{
			name: "Private Key File Not Found",
			yamlContent: `
node:
  id: "node1"
  raft_bind_address: "127.0.0.1:8001"
raft:
  data_dir: "/tmp/data"
signing:
  private_key_path: "` + invalidKeyPath + `" 
scheme: "ed25519"
`,
			expectErrorContains: "signing private key file not found",
		},
		{
			name: "Negative Raft Election Timeout",
			yamlContent: `
node:
  id: "node1"
  raft_bind_address: "127.0.0.1:8001"
raft:
  data_dir: "/tmp/data"
  election_timeout: "-1s"
  heartbeat_timeout: "100ms"
  snapshot_interval: "1m"
signing:
  private_key_path: "` + dummyKeyPath + `"
`,
			expectErrorContains: "raft.election_timeout must be a positive duration",
		},
		{
			name: "Zero Raft Heartbeat Timeout",
			yamlContent: `
node:
  id: "node1"
  raft_bind_address: "127.0.0.1:8001"
raft:
  data_dir: "/tmp/data"
  election_timeout: "1s"
  heartbeat_timeout: "0ms"
  snapshot_interval: "1m"
signing:
  private_key_path: "` + dummyKeyPath + `"
`,
			expectErrorContains: "raft.heartbeat_timeout must be a positive duration",
		},
		{
			name: "Non-positive Raft Snapshot Interval",
			yamlContent: `
node:
  id: "node1"
  raft_bind_address: "127.0.0.1:8001"
raft:
  data_dir: "/tmp/data"
  election_timeout: "1s"
  heartbeat_timeout: "100ms"
  snapshot_interval: "0s"
signing:
  private_key_path: "` + dummyKeyPath + `"
`,
			expectErrorContains: "raft.snapshot_interval must be a positive duration",
		},
		{
			name: "Execution type fullnode, endpoint missing",
			yamlContent: `
node:
  id: "node1"
  raft_bind_address: "127.0.0.1:8001"
raft:
  data_dir: "/tmp/data"
signing:
  private_key_path: "` + dummyKeyPath + `"
execution:
  enabled: true
  type: "fullnode"
  # fullnode_endpoint: is missing
`,
			expectErrorContains: "execution.fullnode_endpoint is required when execution.type is 'fullnode'",
		},
		{
			name: "Execution enabled, timeout zero",
			yamlContent: `
node:
  id: "node1"
  raft_bind_address: "127.0.0.1:8001"
raft:
  data_dir: "/tmp/data"
signing:
  private_key_path: "` + dummyKeyPath + `"
execution:
  enabled: true
  type: "noop"
  timeout: "0s"
`,
			expectErrorContains: "execution.timeout must be a positive duration when execution is enabled",
		},
		{
			name: "Execution enabled, timeout negative",
			yamlContent: `
node:
  id: "node1"
  raft_bind_address: "127.0.0.1:8001"
raft:
  data_dir: "/tmp/data"
signing:
  private_key_path: "` + dummyKeyPath + `"
execution:
  enabled: true
  type: "noop"
  timeout: "-5s"
`,
			expectErrorContains: "execution.timeout must be a positive duration when execution is enabled",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpConfigFile := createTempConfigFile(t, tc.yamlContent)
			_, err := LoadConfig(tmpConfigFile)

			require.Error(t, err, "LoadConfig should fail for this case")
			assert.Contains(t, err.Error(), tc.expectErrorContains, "Error message mismatch")
		})
	}
}
