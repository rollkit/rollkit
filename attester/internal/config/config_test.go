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
	dummyKeyPath := createDummyFile(t, "test_priv_key.pem")

	validYaml := `
node:
  id: "test-node-1"
  raft_bind_address: "127.0.0.1:8001"

raft:
  data_dir: "/tmp/attester-data"
  peers:
    - "127.0.0.1:8002"
    - "127.0.0.1:8003"
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
	assert.Equal(t, []string{"127.0.0.1:8002", "127.0.0.1:8003"}, cfg.Raft.Peers)
	assert.False(t, cfg.Raft.BootstrapCluster)
	assert.Equal(t, "600ms", cfg.Raft.ElectionTimeout)                   // Check original string
	assert.Equal(t, 600*time.Millisecond, cfg.Raft.GetElectionTimeout()) // Check parsed duration
	assert.Equal(t, "60ms", cfg.Raft.HeartbeatTimeout)
	assert.Equal(t, 60*time.Millisecond, cfg.Raft.GetHeartbeatTimeout())
	assert.Equal(t, "3m", cfg.Raft.SnapshotInterval)
	assert.Equal(t, 3*time.Minute, cfg.Raft.GetSnapshotInterval())
	assert.Equal(t, uint64(1000), cfg.Raft.SnapshotThreshold)
	assert.Equal(t, "127.0.0.1:9001", cfg.GRPC.ListenAddress)
	assert.Equal(t, dummyKeyPath, cfg.Signing.PrivateKeyPath)
	assert.Equal(t, "ed25519", cfg.Signing.Scheme)
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("nonexistent/path/to/config.yaml")
	require.Error(t, err, "LoadConfig should fail if config file doesn't exist")
	// Check if the error is specifically a file not found error (optional, depends on desired error precision)
	// require.ErrorIs(t, err, os.ErrNotExist) // Or check for viper.ConfigFileNotFoundError if appropriate
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
	assert.Contains(t, err.Error(), "invalid raft.election_timeout", "Error message should indicate invalid duration")
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
