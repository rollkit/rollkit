package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// RollkitToml is the filename for the rollkit configuration file.
const RollkitToml = "rollkit.toml"

// TomlConfig is the configuration read from rollkit.toml
type TomlConfig struct {
	Entrypoint string          `toml:"entrypoint"`
	Chain      ChainTomlConfig `toml:"chain"`

	RootDir string
}

// ChainTomlConfig is the configuration for the chain section of rollkit.toml
type ChainTomlConfig struct {
	ConfigDir string `toml:"config_dir"`
}

// ReadToml reads the TOML configuration from the rollkit.toml file and returns the parsed TomlConfig.
func ReadToml() (TomlConfig, error) {
	var config TomlConfig
	startDir, err := os.Getwd()
	if err != nil {
		return config, fmt.Errorf("error getting current directory: %w", err)
	}

	configPath, err := findConfigFile(startDir)
	if err != nil {
		return config, err
	}

	if _, err := toml.DecodeFile(configPath, &config); err != nil {
		return config, fmt.Errorf("error reading %s: %w", configPath, err)
	}

	config.RootDir = filepath.Dir(configPath)

	return config, nil
}

func findConfigFile(startDir string) (string, error) {
	dir := startDir
	for {
		configPath := filepath.Join(dir, RollkitToml)
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}

		parentDir := filepath.Dir(dir)
		if parentDir == dir {
			break
		}
		dir = parentDir
	}
	return "", fmt.Errorf("no %s found", RollkitToml)
}
