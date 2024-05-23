package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFindConfigFile(t *testing.T) {
	t.Run("finds config file in current directory", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, RollkitToml)
		err := os.WriteFile(configPath, []byte{}, 0644)
		require.NoError(t, err)

		foundPath, err := findConfigFile(dir)
		require.NoError(t, err)
		require.Equal(t, configPath, foundPath)
	})

	t.Run("finds config file in parent directory", func(t *testing.T) {
		parentDir := t.TempDir()
		dir := filepath.Join(parentDir, "child")
		err := os.Mkdir(dir, 0755)
		require.NoError(t, err)

		configPath := filepath.Join(parentDir, RollkitToml)
		err = os.WriteFile(configPath, []byte{}, 0644)
		require.NoError(t, err)

		foundPath, err := findConfigFile(dir)
		require.NoError(t, err)
		require.Equal(t, configPath, foundPath)
	})

	t.Run("returns error if config file not found", func(t *testing.T) {
		_, err := findConfigFile(t.TempDir())
		require.Error(t, err)
	})
}

func TestReadToml(t *testing.T) {
	t.Run("reads TOML configuration from file", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, RollkitToml)
		err := os.WriteFile(configPath, []byte(`
entrypoint = "./cmd/gm/main.go"

[chain]
config_dir = "config"
`), 0644)
		require.NoError(t, err)

		err = os.Chdir(dir)
		config, err := ReadToml()
		require.NoError(t, err)
		require.Equal(t, TomlConfig{
			Entrypoint: "./cmd/gm/main.go",
			Chain: ChainTomlConfig{
				ConfigDir: "config",
			},
			RootDir: dir,
		}, config)

		// cleanup
		err = os.Remove(configPath)
	})

	t.Run("returns error if config file not found", func(t *testing.T) {
		dir := t.TempDir()
		err := os.Chdir(dir)

		_, err = ReadToml()
		require.Error(t, err)
	})

	t.Run("sets RootDir even if empty toml", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, RollkitToml)
		err := os.WriteFile(configPath, []byte{}, 0644)
		require.NoError(t, err)

		err = os.Chdir(dir)
		config, err := ReadToml()
		require.NoError(t, err)

		// check that config is empty
		require.Equal(t, TomlConfig{RootDir: dir}, config)

		// cleanup
		err = os.Remove(configPath)
	})

	t.Run("returns error if config file cannot be decoded", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, RollkitToml)
		err := os.WriteFile(configPath, []byte(`
blablabla
`), 0644)

		err = os.Chdir(dir)
		_, err = ReadToml()
		require.Error(t, err)

		// cleanup
		err = os.Remove(configPath)
	})
}
