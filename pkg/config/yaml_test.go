package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestYamlConfigOperations(t *testing.T) {
	testCases := []struct {
		name     string
		setup    func(t *testing.T, dir string) *Config
		validate func(t *testing.T, cfg *Config)
	}{
		{
			name: "Write and read custom config values",
			setup: func(t *testing.T, dir string) *Config {
				cfg := DefaultConfig
				cfg.RootDir = dir
				cfg.P2P.ListenAddress = DefaultConfig.P2P.ListenAddress
				cfg.P2P.Seeds = "seed1.example.com:26656,seed2.example.com:26656"

				require.NoError(t, cfg.SaveAsYaml())

				return &cfg
			},
			validate: func(t *testing.T, cfg *Config) {
				require.Equal(t, DefaultConfig.P2P.ListenAddress, cfg.P2P.ListenAddress)
				require.Equal(t, "seed1.example.com:26656,seed2.example.com:26656", cfg.P2P.Seeds)
			},
		},
		{
			name: "Initialize default config values",
			setup: func(t *testing.T, dir string) *Config {
				cfg := DefaultConfig
				cfg.RootDir = dir

				require.NoError(t, cfg.SaveAsYaml())

				return &cfg
			},
			validate: func(t *testing.T, cfg *Config) {
				require.Equal(t, DefaultConfig.P2P.ListenAddress, cfg.P2P.ListenAddress)
				require.Equal(t, DefaultConfig.P2P.Seeds, cfg.P2P.Seeds)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a temporary directory for each test case
			tempDir := t.TempDir()

			path := filepath.Join(tempDir, "config")

			// Setup the test case and write the initial config
			tc.setup(t, tempDir)

			// Verify the config file exists
			configPath := filepath.Join(path, ConfigName)
			_, err := os.Stat(configPath)
			require.NoError(t, err, "Config file should exist")

			// Read the config
			cmd := &cobra.Command{Use: "test"}
			AddFlags(cmd)
			AddGlobalFlags(cmd, "test")
			cfg, err := Load(cmd)
			require.NoError(t, err)

			// Validate the config
			tc.validate(t, &cfg)
		})
	}
}
