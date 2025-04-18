package config

import (
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
				cfg.P2P.Peers = "seed1.example.com:26656,seed2.example.com:26656"

				require.NoError(t, cfg.SaveAsYaml())

				return &cfg
			},
			validate: func(t *testing.T, cfg *Config) {
				require.Equal(t, DefaultConfig.P2P.ListenAddress, cfg.P2P.ListenAddress)
				require.Equal(t, "seed1.example.com:26656,seed2.example.com:26656", cfg.P2P.Peers)
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
				require.Equal(t, DefaultConfig.P2P.Peers, cfg.P2P.Peers)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()

			// Setup the test case and write the initial config
			tc.setup(t, tempDir)

			// Read the config
			cmd := &cobra.Command{Use: "test"}
			AddFlags(cmd)
			AddGlobalFlags(cmd, "")
			args := []string{"--home=" + tempDir}
			cmd.SetArgs(args)
			require.NoError(t, cmd.ParseFlags(args))

			cfg, err := Load(cmd)
			require.NoError(t, err)
			require.NoError(t, cfg.Validate())

			// Validate the config
			tc.validate(t, &cfg)
		})
	}
}
