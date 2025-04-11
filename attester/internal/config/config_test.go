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

	// Assertions for new sections (assuming defaults or example values were set)
	// Add specific values to the validYaml if testing non-defaults
	// assert.Equal(t, "http://sequencer:1234", cfg.Network.SequencerSigEndpoint) // Example if set in YAML
	// assert.Equal(t, 2, cfg.Aggregator.QuorumThreshold) // Example if set in YAML
	// assert.Equal(t, map[string]string{"attester1": "addr1"}, cfg.Aggregator.Attesters) // Example if set in YAML
	// assert.True(t, cfg.Execution.Enabled) // Example if set in YAML
	// assert.Equal(t, "fullnode", cfg.Execution.Type) // Example if set in YAML
	// assert.Equal(t, "http://fullnode:5678", cfg.Execution.FullnodeEndpoint) // Example if set in YAML
	// assert.Equal(t, 5*time.Second, cfg.Execution.Timeout) // Example if set in YAML
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

func TestLoadConfig_InvalidYaml(t *testing.T) {
	invalidYaml := `
node:
  id: "bad-yaml"
 raft_bind_address: "-" # Bad indentation
`
	tmpConfigFile := createTempConfigFile(t, invalidYaml)

	_, err := LoadConfig(tmpConfigFile)
	require.Error(t, err, "LoadConfig should fail for invalid YAML syntax")
}

func TestLoadConfig_MissingRequiredField_NodeID(t *testing.T) {
	dummyKeyPath := createDummyFile(t, "key.pem")
	missingNodeID := `
node:
  raft_bind_address: "127.0.0.1:8001"

raft:
  data_dir: "/tmp/data"
  election_timeout: "1s"
  heartbeat_timeout: "100ms"
  snapshot_interval: "1m"
  snapshot_threshold: 10

grpc:
  listen_address: "127.0.0.1:9001"

signing:
  private_key_path: "` + dummyKeyPath + `"
  scheme: "ed25519"
`
	tmpConfigFile := createTempConfigFile(t, missingNodeID)
	_, err := LoadConfig(tmpConfigFile)
	require.Error(t, err, "LoadConfig should fail if node.id is missing")
	assert.Contains(t, err.Error(), "node.id is required", "Error message should indicate missing node.id")
}

func TestLoadConfig_MissingRequiredField_SigningKeyPath(t *testing.T) {
	missingKeyPath := `
node:
  id: "node1"
  raft_bind_address: "127.0.0.1:8001"

raft:
  data_dir: "/tmp/data"
  election_timeout: "1s"
  heartbeat_timeout: "100ms"
  snapshot_interval: "1m"
  snapshot_threshold: 10

grpc:
  listen_address: "127.0.0.1:9001"

signing:
  scheme: "ed25519"
`
	tmpConfigFile := createTempConfigFile(t, missingKeyPath)
	_, err := LoadConfig(tmpConfigFile)
	require.Error(t, err, "LoadConfig should fail if signing.private_key_path is missing")
	assert.Contains(t, err.Error(), "signing.private_key_path is required", "Error message should indicate missing key path")
}

func TestLoadConfig_MissingRequiredField_RaftDataDir(t *testing.T) {
	dummyKeyPath := createDummyFile(t, "key.pem")
	missingDataDir := `
node:
  id: "node1"
  raft_bind_address: "127.0.0.1:8001"

raft:
  election_timeout: "1s"
  heartbeat_timeout: "100ms"
  snapshot_interval: "1m"
  snapshot_threshold: 10

grpc:
  listen_address: "127.0.0.1:9001"

signing:
  private_key_path: "` + dummyKeyPath + `"
  scheme: "ed25519"
`
	tmpConfigFile := createTempConfigFile(t, missingDataDir)
	_, err := LoadConfig(tmpConfigFile)
	require.Error(t, err, "LoadConfig should fail if raft.data_dir is missing")
	assert.Contains(t, err.Error(), "raft.data_dir is required", "Error message should indicate missing data dir")
}

func TestLoadConfig_InvalidDuration(t *testing.T) {
	dummyKeyPath := createDummyFile(t, "key.pem")
	invalidDuration := `
node:
  id: "node1"
  raft_bind_address: "127.0.0.1:8001"

raft:
  data_dir: "/tmp/data"
  election_timeout: "not-a-duration" # Invalid value
  heartbeat_timeout: "100ms"
  snapshot_interval: "1m"
  snapshot_threshold: 10

grpc:
  listen_address: "127.0.0.1:9001"

signing:
  private_key_path: "` + dummyKeyPath + `"
  scheme: "ed25519"
`
	tmpConfigFile := createTempConfigFile(t, invalidDuration)
	_, err := LoadConfig(tmpConfigFile)
	require.Error(t, err, "LoadConfig should fail for invalid duration string")
	assert.Contains(t, err.Error(), "failed to unmarshal config", "Error message should indicate unmarshal failure due to duration")
}

func TestLoadConfig_PrivateKeyNotFound(t *testing.T) {
	missingKeyPath := `
node:
  id: "node1"
  raft_bind_address: "127.0.0.1:8001"

raft:
  data_dir: "/tmp/data"
  election_timeout: "1s"
  heartbeat_timeout: "100ms"
  snapshot_interval: "1m"
  snapshot_threshold: 10

grpc:
  listen_address: "127.0.0.1:9001"

signing:
  private_key_path: "/path/to/nonexistent/key.pem" # Key file does not exist
  scheme: "ed25519"
`
	tmpConfigFile := createTempConfigFile(t, missingKeyPath)
	_, err := LoadConfig(tmpConfigFile)
	require.Error(t, err, "LoadConfig should fail if private key file does not exist")
	assert.Contains(t, err.Error(), "signing private key file not found", "Error message should indicate missing key file")
}

func TestLoadConfig_InvalidRaftDurations(t *testing.T) {
	dummyKeyPath := createDummyFile(t, "key.pem")
	tests := []struct {
		name        string
		yamlContent string
		expectError string
	}{
		{
			name: "Negative Election Timeout",
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
`, // grpc and others omitted for brevity, relying on defaults/validation order
			expectError: "raft.election_timeout must be a positive duration",
		},
		{
			name: "Zero Heartbeat Timeout",
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
			expectError: "raft.heartbeat_timeout must be a positive duration",
		},
		{
			name: "Non-positive Snapshot Interval",
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
			expectError: "raft.snapshot_interval must be a positive duration",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpConfigFile := createTempConfigFile(t, tc.yamlContent)
			_, err := LoadConfig(tmpConfigFile)
			require.Error(t, err, "LoadConfig should fail for invalid raft duration")
			assert.Contains(t, err.Error(), tc.expectError, "Error message mismatch")
		})
	}
}

func TestLoadConfig_InvalidExecutionConfig(t *testing.T) {
	dummyKeyPath := createDummyFile(t, "key.pem")
	tests := []struct {
		name        string
		yamlContent string
		expectError string
	}{
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
			expectError: "execution.fullnode_endpoint is required when execution.type is 'fullnode'",
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
			expectError: "execution.timeout must be a positive duration when execution is enabled",
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
			expectError: "execution.timeout must be a positive duration when execution is enabled",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpConfigFile := createTempConfigFile(t, tc.yamlContent)
			_, err := LoadConfig(tmpConfigFile)
			require.Error(t, err, "LoadConfig should fail for invalid execution config")
			assert.Contains(t, err.Error(), tc.expectError, "Error message mismatch")
		})
	}
}
