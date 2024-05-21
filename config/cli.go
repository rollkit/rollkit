package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

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
		return config, fmt.Errorf("error reading rollkit.toml: %v", err)
	}

	config.RootDir = filepath.Dir(configPath)

	return config, nil
}

func findConfigFile(startDir string) (string, error) {
	dir := startDir
	for {
		configPath := filepath.Join(dir, "rollkit.toml")
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}

		parentDir := filepath.Dir(dir)
		if parentDir == dir {
			break
		}
		dir = parentDir
	}
	return "", fmt.Errorf("no rollkit.toml found")
}
