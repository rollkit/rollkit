package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
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

		configPath := filepath.Join(dir, RollkitConfigToml)
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

		configPath := filepath.Join(parentDir, RollkitConfigToml)
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

		configPath := filepath.Join(dir, RollkitConfigToml)
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

		configPath := filepath.Join(dir, RollkitConfigToml)
		err = os.WriteFile(configPath, []byte(`
entrypoint = "./cmd/app/main.go"

[chain]
config_dir = "custom-config"

[rollkit]
aggregator = true
block_time = "2s"
lazy_aggregator = true
sequencer_address = "custom-sequencer:50051"
sequencer_rollup_id = "custom-rollup"

[da]
address = "http://custom-da:26658"
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
		expectedConfig.Node.Aggregator = true
		expectedConfig.Node.BlockTime = 2 * time.Second
		expectedConfig.DA.Address = "http://custom-da:26658"
		expectedConfig.Node.LazyAggregator = true
		expectedConfig.Node.SequencerAddress = "custom-sequencer:50051"
		expectedConfig.Node.SequencerRollupID = "custom-rollup"

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

		configPath := filepath.Join(dir, RollkitConfigToml)
		err = os.WriteFile(configPath, []byte{}, 0600)
		require.NoError(t, err)

		require.NoError(t, os.Chdir(dir))
		config, err := ReadToml()
		require.NoError(t, err)

		// Create expected config with default values
		expectedConfig := DefaultNodeConfig

		// Update expected RootDir to match the test directory
		expectedConfig.RootDir = dir

		// Update expected Chain.ConfigDir to match the test directory
		if expectedConfig.Chain.ConfigDir != "" && !filepath.IsAbs(expectedConfig.Chain.ConfigDir) {
			expectedConfig.Chain.ConfigDir = filepath.Join(dir, expectedConfig.Chain.ConfigDir)
		}

		// check that config has default values with updated RootDir
		require.Equal(t, expectedConfig, config)
	})

	t.Run("returns error if config file cannot be decoded", func(t *testing.T) {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)

		configPath := filepath.Join(dir, RollkitConfigToml)
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

			// Create a config with appropriate values
			config := DefaultNodeConfig
			config.RootDir = dir

			// Set custom values if needed
			if tc.useCustomValues {
				config.Entrypoint = "./cmd/custom/main.go"
				config.Chain.ConfigDir = "custom-config"

				// Set various Rollkit config values to test different types
				config.Node.Aggregator = true
				config.Node.Light = true
				config.Node.LazyAggregator = true
				config.Node.BlockTime = 5 * time.Second
				config.DA.Address = "http://custom-da:26658"
				config.Node.SequencerAddress = "custom-sequencer:50051"
				config.Node.SequencerRollupID = "custom-rollup"
			} else {
				// For default values test, ensure ConfigDir is set to the default value
				config.Chain.ConfigDir = DefaultConfigDir
			}

			// Write the config to a TOML file
			err = WriteTomlConfig(config)
			require.NoError(t, err)

			// Verify the file was created
			configPath := filepath.Join(dir, RollkitConfigToml)
			_, err = os.Stat(configPath)
			require.NoError(t, err)

			readConfig, err := readTomlFromPath(configPath)
			require.NoError(t, err)

			// Create expected config with appropriate values
			expectedConfig := DefaultNodeConfig
			expectedConfig.RootDir = dir

			if tc.useCustomValues {
				expectedConfig.Entrypoint = "./cmd/custom/main.go"
				expectedConfig.Chain.ConfigDir = filepath.Join(dir, "custom-config")

				// Set the same custom values as above
				expectedConfig.Node.Aggregator = true
				expectedConfig.Node.Light = true
				expectedConfig.Node.LazyAggregator = true
				expectedConfig.Node.BlockTime = 5 * time.Second
				expectedConfig.DA.Address = "http://custom-da:26658"
				expectedConfig.Node.SequencerAddress = "custom-sequencer:50051"
				expectedConfig.Node.SequencerRollupID = "custom-rollup"
			} else {
				// For default values test, set the expected ConfigDir to match what ReadToml will return
				expectedConfig.Chain.ConfigDir = filepath.Join(dir, DefaultConfigDir)
			}

			// Verify the read config matches the expected config
			require.Equal(t, expectedConfig, readConfig)

			// Verify the file content if needed
			if tc.verifyFileContent {
				content, err := os.ReadFile(configPath) //nolint:gosec // This is a test file with a controlled path
				require.NoError(t, err)
				require.NotEmpty(t, content)
			}
		})
	}
}

// readTomlFromPath reads a TOML file from the given path without relying on os.Getwd(), needed for CI
func readTomlFromPath(configPath string) (config RollkitConfig, err error) {
	// Create a new Viper instance to avoid conflicts with any global Viper
	v := viper.New()

	// Set default values
	config = DefaultNodeConfig

	// Configure Viper to read the TOML file
	v.SetConfigFile(configPath)

	// Read the config file
	if err := v.ReadInConfig(); err != nil {
		return config, fmt.Errorf("error reading config file: %w", err)
	}

	// Get the directory of the config file to set as RootDir
	configDir := filepath.Dir(configPath)
	config.RootDir = configDir

	// Unmarshal the config file into the config struct
	if err := v.Unmarshal(&config, func(c *mapstructure.DecoderConfig) {
		c.TagName = "mapstructure"
		c.DecodeHook = mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		)
	}); err != nil {
		return config, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Make ConfigDir absolute if it's not already
	if config.Chain.ConfigDir != "" && !filepath.IsAbs(config.Chain.ConfigDir) {
		config.Chain.ConfigDir = filepath.Join(configDir, config.Chain.ConfigDir)
	}

	return config, nil
}
