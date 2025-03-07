package config

import (
	"os"
	"path/filepath"
	"testing"

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
