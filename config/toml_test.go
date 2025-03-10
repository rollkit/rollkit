package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFindentrypoint(t *testing.T) {
	t.Run("finds entrypoint in current directory", func(t *testing.T) {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)

		require.NoError(t, os.Chdir(dir))

		entrypointPath := filepath.Join(dir, "main.go")
		err = os.WriteFile(entrypointPath, []byte{}, 0600)
		require.NoError(t, err)

		dirName, fullDirPath := FindEntrypoint()
		require.Equal(t, dir, dirName)
		require.Equal(t, entrypointPath, fullDirPath)
	})

	t.Run("returns error if entrypoint not found", func(t *testing.T) {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)

		require.NoError(t, os.Chdir(dir))

		dirName, fullDirPath := FindEntrypoint()
		require.Empty(t, dirName)
		require.Empty(t, fullDirPath)
	})

	t.Run("finds entrypoint in subdirectory", func(t *testing.T) {
		parentDir, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)

		dir := filepath.Join(parentDir, "child")
		err = os.Mkdir(dir, 0750)
		require.NoError(t, err)

		require.NoError(t, os.Chdir(dir))

		entrypointPath := filepath.Join(dir, "main.go")
		err = os.WriteFile(entrypointPath, []byte{}, 0600)
		require.NoError(t, err)

		dirName, fullDirPath := FindEntrypoint()
		require.Equal(t, dir, dirName)
		require.Equal(t, entrypointPath, fullDirPath)
	})
}

func TestFindConfigFile(t *testing.T) {
	t.Run("finds config file in current directory", func(t *testing.T) {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)

		configPath := filepath.Join(dir, RollkitToml)
		err = os.WriteFile(configPath, []byte{}, 0600)
		require.NoError(t, err)

		foundPath, err := findConfigFile(dir)
		require.NoError(t, err)
		require.Equal(t, configPath, foundPath)
	})

	t.Run("finds config file in parent directory", func(t *testing.T) {
		parentDir, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)

		dir := filepath.Join(parentDir, "child")
		err = os.Mkdir(dir, 0750)
		require.NoError(t, err)

		configPath := filepath.Join(parentDir, RollkitToml)
		err = os.WriteFile(configPath, []byte{}, 0600)
		require.NoError(t, err)

		foundPath, err := findConfigFile(dir)
		require.NoError(t, err)
		require.Equal(t, configPath, foundPath)
	})

	t.Run("returns error if config file not found", func(t *testing.T) {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)

		_, err = findConfigFile(dir)
		require.Error(t, err)
	})
}

func TestReadToml(t *testing.T) {
	t.Run("reads TOML configuration from file", func(t *testing.T) {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)

		configPath := filepath.Join(dir, RollkitToml)
		err = os.WriteFile(configPath, []byte(`
entrypoint = "./cmd/gm/main.go"

[chain]
config_dir = "config"
`), 0600)
		require.NoError(t, err)

		require.NoError(t, os.Chdir(dir))
		config, err := ReadToml()
		require.NoError(t, err)

		// Create expected config with default values
		expectedConfig := DefaultNodeConfig
		expectedConfig.RootDir = dir
		expectedConfig.Entrypoint = "./cmd/gm/main.go"
		expectedConfig.Chain.ConfigDir = filepath.Join(dir, "config")

		require.Equal(t, expectedConfig, config)
	})

	t.Run("loads nodeconfig values from TOML file", func(t *testing.T) {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)

		configPath := filepath.Join(dir, RollkitToml)
		err = os.WriteFile(configPath, []byte(`
entrypoint = "./cmd/app/main.go"

[chain]
config_dir = "custom-config"

[rollkit]
aggregator = true
block_time = "2s"
da_address = "http://custom-da:26658"
lazy_aggregator = true
sequencer_address = "custom-sequencer:50051"
sequencer_rollup_id = "custom-rollup"
`), 0600)
		require.NoError(t, err)

		require.NoError(t, os.Chdir(dir))
		config, err := ReadToml()
		require.NoError(t, err)

		// Create expected config with default values
		expectedConfig := DefaultNodeConfig
		expectedConfig.RootDir = dir
		expectedConfig.Entrypoint = "./cmd/app/main.go"
		expectedConfig.Chain.ConfigDir = filepath.Join(dir, "custom-config")

		// These values should be loaded from the TOML file
		// Only set the values that are actually in the TOML file
		expectedConfig.Rollkit.Aggregator = true
		expectedConfig.Rollkit.BlockTime = 2 * time.Second
		expectedConfig.Rollkit.DAAddress = "http://custom-da:26658"
		expectedConfig.Rollkit.LazyAggregator = true
		expectedConfig.Rollkit.SequencerAddress = "custom-sequencer:50051"
		expectedConfig.Rollkit.SequencerRollupID = "custom-rollup"

		require.Equal(t, expectedConfig, config)
	})

	t.Run("returns error if config file not found", func(t *testing.T) {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)

		require.NoError(t, os.Chdir(dir))

		_, err = ReadToml()
		require.Error(t, err)
	})

	t.Run("sets RootDir even if empty toml", func(t *testing.T) {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)

		configPath := filepath.Join(dir, RollkitToml)
		err = os.WriteFile(configPath, []byte{}, 0600)
		require.NoError(t, err)

		require.NoError(t, os.Chdir(dir))
		config, err := ReadToml()
		require.NoError(t, err)

		// Create expected config with default values
		expectedConfig := DefaultNodeConfig
		expectedConfig.RootDir = dir
		// When reading an empty TOML file, the Chain.ConfigDir should be empty
		expectedConfig.Chain.ConfigDir = ""

		// check that config has default values with updated RootDir
		require.Equal(t, expectedConfig, config)
	})

	t.Run("returns error if config file cannot be decoded", func(t *testing.T) {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)

		configPath := filepath.Join(dir, RollkitToml)
		require.NoError(t, os.WriteFile(configPath, []byte(`
blablabla
`), 0600))

		require.NoError(t, os.Chdir(dir))
		_, err = ReadToml()
		require.Error(t, err)
	})
}

func TestTomlConfigOperations(t *testing.T) {
	testCases := []struct {
		name              string
		useCustomValues   bool
		verifyFileContent bool
	}{
		{
			name:              "Write and read custom config values",
			useCustomValues:   true,
			verifyFileContent: false,
		},
		{
			name:              "Initialize default config values",
			useCustomValues:   false,
			verifyFileContent: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a temporary directory for testing
			dir, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)

			// Save current directory to restore it later
			originalDir, err := os.Getwd()
			require.NoError(t, err)

			// Ensure we change back to the original directory when the test completes
			defer func() {
				err := os.Chdir(originalDir)
				if err != nil {
					t.Logf("Failed to change back to original directory: %v", err)
				}
			}()

			// Change to the temporary directory
			err = os.Chdir(dir)
			require.NoError(t, err)

			// Create a config with appropriate values
			config := DefaultNodeConfig
			config.RootDir = dir

			// Set custom values if needed
			if tc.useCustomValues {
				config.Entrypoint = "./cmd/custom/main.go"
				config.Chain.ConfigDir = "custom-config"

				// Set various Rollkit config values to test different types
				config.Rollkit.Aggregator = true
				config.Rollkit.Light = true
				config.Rollkit.LazyAggregator = true
				config.Rollkit.BlockTime = 5 * time.Second
				config.Rollkit.DAAddress = "http://custom-da:26658"
				config.Rollkit.SequencerAddress = "custom-sequencer:50051"
				config.Rollkit.SequencerRollupID = "custom-rollup"
			} else {
				// For default values test, ensure ConfigDir is set to the default value
				config.Chain.ConfigDir = DefaultConfigDir
			}

			// Write the config to a TOML file
			err = WriteTomlConfig(config)
			require.NoError(t, err)

			// Verify the file was created
			configPath := filepath.Join(dir, RollkitToml)
			_, err = os.Stat(configPath)
			require.NoError(t, err)

			// Read the config back from the file
			readConfig, err := ReadToml()
			require.NoError(t, err)

			// Create expected config with appropriate values
			expectedConfig := DefaultNodeConfig
			expectedConfig.RootDir = dir

			if tc.useCustomValues {
				expectedConfig.Entrypoint = "./cmd/custom/main.go"
				expectedConfig.Chain.ConfigDir = filepath.Join(dir, "custom-config")

				// Set the same custom values as above
				expectedConfig.Rollkit.Aggregator = true
				expectedConfig.Rollkit.Light = true
				expectedConfig.Rollkit.LazyAggregator = true
				expectedConfig.Rollkit.BlockTime = 5 * time.Second
				expectedConfig.Rollkit.DAAddress = "http://custom-da:26658"
				expectedConfig.Rollkit.SequencerAddress = "custom-sequencer:50051"
				expectedConfig.Rollkit.SequencerRollupID = "custom-rollup"
			} else {
				// For default values test, set the expected ConfigDir to match what ReadToml will return
				expectedConfig.Chain.ConfigDir = filepath.Join(dir, DefaultConfigDir)
			}

			// Verify the read config matches the expected config
			require.Equal(t, expectedConfig, readConfig)

			// Verify the file content if needed
			if tc.verifyFileContent {
				content, err := os.ReadFile(configPath)
				require.NoError(t, err)
				require.NotEmpty(t, content)
			}
		})
	}
}
