package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const (
	RollkitToml = "rollkit.toml"
)

// TomlConfig is the configuration read from rollkit.toml
func ReadToml() (TomlConfig, error) {
	var config TomlConfig
	startDir, err := os.Getwd()
	if err != nil {
		return config, fmt.Errorf("error getting current directory: %v", err)
	}

	configPath, err := findConfigFile(startDir)
	if err != nil {
		return config, err
	}

	if _, err := toml.DecodeFile(configPath, &config); err != nil {
		return config, fmt.Errorf("error reading %s: %v", configPath, err)
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
