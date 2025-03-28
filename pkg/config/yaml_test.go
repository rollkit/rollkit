package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReadYaml(t *testing.T) {
	testCases := []struct {
		name     string
		setup    func(t *testing.T, dir string) error
		validate func(t *testing.T, cfg Config, err error)
	}{
		{
			name: "reads YAML configuration from file",
			setup: func(t *testing.T, dir string) error {
				cfg := DefaultNodeConfig
				cfg.RootDir = dir
				return WriteYamlConfig(cfg)
			},
			validate: func(t *testing.T, cfg Config, err error) {
				require.NoError(t, err)
				require.Equal(t, DefaultNodeConfig.P2P.ListenAddress, cfg.P2P.ListenAddress)
			},
		},
		{
			name: "loads nodeconfig values from YAML file",
			setup: func(t *testing.T, dir string) error {
				cfg := DefaultNodeConfig
				cfg.RootDir = dir
				cfg.Node.Aggregator = true
				cfg.Node.Light = true
				return WriteYamlConfig(cfg)
			},
			validate: func(t *testing.T, cfg Config, err error) {
				require.NoError(t, err)
				require.True(t, cfg.Node.Aggregator)
				require.True(t, cfg.Node.Light)
			},
		},
		{
			name: "returns error if config file not found",
			setup: func(t *testing.T, dir string) error {
				return nil
			},
			validate: func(t *testing.T, cfg Config, err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "Config File \"rollkit\" Not Found")
			},
		},
		{
			name: "sets RootDir even if empty yaml",
			setup: func(t *testing.T, dir string) error {
				// Create empty YAML file
				return os.WriteFile(filepath.Join(dir, RollkitConfigYaml), []byte(""), 0600)
			},
			validate: func(t *testing.T, cfg Config, err error) {
				require.NoError(t, err)
				require.NotEmpty(t, cfg.RootDir)
			},
		},
		{
			name: "returns error if config file cannot be decoded",
			setup: func(t *testing.T, dir string) error {
				// Create invalid YAML file
				return os.WriteFile(filepath.Join(dir, RollkitConfigYaml), []byte("invalid: yaml: content"), 0600)
			},
			validate: func(t *testing.T, cfg Config, err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "decoding file")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a temporary directory for each test case
			tempDir := t.TempDir()

			// Setup the test case
			err := tc.setup(t, tempDir)
			require.NoError(t, err)

			// Read the config
			cfg, err := ReadYaml(tempDir)

			// Validate the result
			tc.validate(t, cfg, err)
		})
	}
}

func TestYamlConfigOperations(t *testing.T) {
	testCases := []struct {
		name     string
		setup    func(t *testing.T, dir string) *Config
		validate func(t *testing.T, cfg *Config)
	}{
		{
			name: "Write and read custom config values",
			setup: func(t *testing.T, dir string) *Config {
				cfg := DefaultNodeConfig
				cfg.RootDir = dir
				cfg.P2P.ListenAddress = DefaultNodeConfig.P2P.ListenAddress
				cfg.P2P.Seeds = "seed1.example.com:26656,seed2.example.com:26656"

				// Write the config file
				err := WriteYamlConfig(cfg)
				require.NoError(t, err)

				return &cfg
			},
			validate: func(t *testing.T, cfg *Config) {
				require.Equal(t, DefaultNodeConfig.P2P.ListenAddress, cfg.P2P.ListenAddress)
				require.Equal(t, "seed1.example.com:26656,seed2.example.com:26656", cfg.P2P.Seeds)
			},
		},
		{
			name: "Initialize default config values",
			setup: func(t *testing.T, dir string) *Config {
				cfg := DefaultNodeConfig
				cfg.RootDir = dir

				// Write the config file
				err := WriteYamlConfig(cfg)
				require.NoError(t, err)

				return &cfg
			},
			validate: func(t *testing.T, cfg *Config) {
				require.Equal(t, DefaultNodeConfig.P2P.ListenAddress, cfg.P2P.ListenAddress)
				require.Equal(t, DefaultNodeConfig.P2P.Seeds, cfg.P2P.Seeds)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a temporary directory for each test case
			tempDir := t.TempDir()

			// Setup the test case and write the initial config
			tc.setup(t, tempDir)

			// Verify the config file exists
			configPath := filepath.Join(tempDir, RollkitConfigYaml)
			_, err := os.Stat(configPath)
			require.NoError(t, err, "Config file should exist")

			// Read the config back
			cfg, err := ReadYaml(tempDir)
			require.NoError(t, err)

			// Validate the config
			tc.validate(t, &cfg)
		})
	}
}

func TestCreateInitialConfig(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Test creating initial config with default values
	err := CreateInitialConfig(tempDir)
	require.NoError(t, err)

	// Verify the config file exists
	configPath := filepath.Join(tempDir, RollkitConfigYaml)
	_, err = os.Stat(configPath)
	require.NoError(t, err)

	// Read the config back
	cfg, err := ReadYaml(tempDir)
	require.NoError(t, err)

	// Verify root directory is set correctly
	require.Equal(t, tempDir, cfg.RootDir)

	// Test creating config with customizations
	tempDir2 := t.TempDir()
	err = CreateInitialConfig(tempDir2, func(cfg *Config) {
		cfg.Node.Aggregator = true
		cfg.Node.BlockTime.Duration = 5 * time.Second
	})
	require.NoError(t, err)

	// Read the customized config
	cfg, err = ReadYaml(tempDir2)
	require.NoError(t, err)

	// Verify customizations were applied
	require.True(t, cfg.Node.Aggregator)
	require.Equal(t, 5*time.Second, cfg.Node.BlockTime.Duration)

	// Test error when config file already exists
	err = CreateInitialConfig(tempDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "file already exists")
}
