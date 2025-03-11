package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

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

			readConfig, err := readTomlFromPath(configPath)
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
				content, err := os.ReadFile(configPath) //nolint:gosec // This is a test file with a controlled path
				require.NoError(t, err)
				require.NotEmpty(t, content)
			}
		})
	}
}

// readTomlFromPath reads a TOML file from the given path without relying on os.Getwd(), needed for CI
func readTomlFromPath(configPath string) (config NodeConfig, err error) {
	// Set the default values
	config = DefaultNodeConfig

	// Configure Viper to read the TOML file
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("toml")

	// Read the configuration file
	if err = v.ReadInConfig(); err != nil {
		err = fmt.Errorf("%w decoding file %s: %w", ErrReadToml, configPath, err)
		return
	}

	// Check if the file is empty
	fileInfo, err := os.Stat(configPath)
	if err != nil {
		err = fmt.Errorf("%w getting file info: %w", ErrReadToml, err)
		return
	}

	isEmptyFile := fileInfo.Size() == 0

	// Unmarshal directly into NodeConfig
	if err = v.Unmarshal(&config); err != nil {
		err = fmt.Errorf("%w unmarshaling config: %w", ErrReadToml, err)
		return
	}

	// Set the root directory
	config.RootDir = filepath.Dir(configPath)

	// Add configPath to chain.ConfigDir if it is a relative path and the file is not empty
	if !isEmptyFile && config.Chain.ConfigDir != "" && !filepath.IsAbs(config.Chain.ConfigDir) {
		config.Chain.ConfigDir = filepath.Join(config.RootDir, config.Chain.ConfigDir)
	} else if isEmptyFile {
		// When reading an empty TOML file, the Chain.ConfigDir should be empty
		config.Chain.ConfigDir = ""
	}

	return
}
