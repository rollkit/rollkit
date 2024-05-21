package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/BurntSushi/toml"
	cometos "github.com/cometbft/cometbft/libs/os"
	rollconf "github.com/rollkit/rollkit/config"
)

const (
	rollupBinEntrypoint = "entrypoint"
)

var (
	// rollkit.toml file configuration
	rollkitConfig rollconf.TomlConfig
)

func InterceptCommand() error {
	var err error
	rollkitConfig, err = readTomlConfig()
	if err != nil {
		return err
	}

	if rollkitConfig.Entrypoint != "" {
		flags := []string{}
		if len(os.Args) >= 2 {
			flags = os.Args[1:]
		}
		return runEntrypoint(rollkitConfig, flags)
	}

	return nil
}

func readTomlConfig() (rollconf.TomlConfig, error) {
	var config rollconf.TomlConfig
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

func runEntrypoint(rollkitConfig rollconf.TomlConfig, args []string) error {
	entrypointSourceFile := filepath.Join(rollkitConfig.RootDir, rollkitConfig.Entrypoint)
	entrypointBinaryFile := filepath.Join(rollkitConfig.RootDir, rollupBinEntrypoint)

	if !cometos.FileExists(entrypointBinaryFile) {
		if !cometos.FileExists(entrypointSourceFile) {
			return fmt.Errorf("rollkit: no such entrypoint file: %s", entrypointSourceFile)
		}

		// try to build the entrypoint
		var buildArgs []string
		buildArgs = append(buildArgs, "build")
		buildArgs = append(buildArgs, "-o", entrypointBinaryFile)
		buildArgs = append(buildArgs, entrypointSourceFile)
		buildCmd := exec.Command("go", buildArgs...)
		buildCmd.Stdout = os.Stdout
		buildCmd.Stderr = os.Stderr
		if err := buildCmd.Run(); err != nil {
			return fmt.Errorf("rollkit: failed to build entrypoint: %w", err)
		}
	}

	// run the entrypoint
	var runArgs []string
	runArgs = append(runArgs, args...)
	// we have to pass --home flag to the entrypoint, so it reads the correct config root directory
	runArgs = append(runArgs, "--home", rollkitConfig.Chain.ConfigDir)
	fmt.Printf("Running entrypoint: %s %v\n", entrypointBinaryFile, runArgs)
	entrypointCmd := exec.Command(entrypointBinaryFile, runArgs...)
	entrypointCmd.Stdout = os.Stdout
	entrypointCmd.Stderr = os.Stderr

	if err := entrypointCmd.Run(); err != nil {
		return fmt.Errorf("rollkit: failed to run entrypoint: %w", err)
	}

	return nil
}
