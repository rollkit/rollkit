package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	cometos "github.com/cometbft/cometbft/libs/os"
	rollconf "github.com/rollkit/rollkit/config"
)

const (
	rollupBinEntrypoint = "entrypoint"
)

var (
	rollkitConfig rollconf.TomlConfig
)

// InterceptCommand intercepts the command and runs it against the `entrypoint`
// specified in the rollkit.toml configuration file.
func InterceptCommand() error {
	var err error
	rollkitConfig, err = rollconf.ReadToml()
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
