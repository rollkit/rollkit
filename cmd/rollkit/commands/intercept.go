package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	cometos "github.com/cometbft/cometbft/libs/os"
	"github.com/spf13/cobra"

	rollconf "github.com/rollkit/rollkit/config"
)

const rollupBinEntrypoint = "entrypoint"

var (
	rollkitConfig rollconf.TomlConfig

	// ErrRollkitCommand is returned when the command is a rollkit command
	// which we don't want to intercept.
	ErrRollkitCommand = fmt.Errorf("rollkit command")

	// ErrHelpVersion is returned when the user attempts to run help or version.
	ErrHelpVersion = fmt.Errorf("help or version")

	// ErrRunEntrypoint is returned when the entrypoint fails to run.
	ErrRunEntrypoint = fmt.Errorf("run rollup entrypoint")
)

// InterceptCommand intercepts the command and runs it against the `entrypoint`
// specified in the rollkit.toml configuration file.
func InterceptCommand(
	rollkitCommand *cobra.Command,
	readToml func() (rollconf.TomlConfig, error),
	runEntrypoint func(*rollconf.TomlConfig, []string) error,
) error {
	skipCommands := []string{}
	for _, cmd := range rollkitCommand.Commands() {
		skipCommands = append(skipCommands, cmd.Use)
	}

	if len(os.Args) >= 2 {
		// check if user attempted to run a rollkit command
		// if so, we don't want to intercept it except for `start`
		for _, skipCmd := range skipCommands {
			if os.Args[1] == skipCmd && os.Args[1] != "start" {
				return ErrRollkitCommand
			}
		}

		// check if user attempted to run help or version
		switch os.Args[1] {
		case "help", "--help", "h", "-h", "version", "--version", "v", "-v", "toml":
			return ErrHelpVersion
		}
	}

	var err error
	rollkitConfig, err = readToml()
	if err != nil {
		return err
	}

	if rollkitConfig.Entrypoint == "" {
		return fmt.Errorf("no entrypoint specified in %s", rollconf.RollkitToml)
	}

	flags := []string{}
	if len(os.Args) >= 2 {
		flags = os.Args[1:]
	}

	return runEntrypoint(&rollkitConfig, flags)
}

// RunRollupEntrypoint runs the entrypoint specified in the rollkit.toml configuration file.
// If the entrypoint is not built, it will build it first. The entrypoint is built
// in the same directory as the rollkit.toml file. The entrypoint is run with the
// same flags as the original command, but with the `--home` flag set to the config
// directory of the chain specified in the rollkit.toml file. This is so the entrypoint,
// which is a separate binary of the rollup, can read the correct chain configuration files.
func RunRollupEntrypoint(rollkitConfig *rollconf.TomlConfig, args []string) error {
	var entrypointSourceFile string
	if !filepath.IsAbs(rollkitConfig.RootDir) {
		entrypointSourceFile = filepath.Join(rollkitConfig.RootDir, rollkitConfig.Entrypoint)
	} else {
		entrypointSourceFile = rollkitConfig.Entrypoint
	}

	entrypointBinaryFile := filepath.Join(rollkitConfig.RootDir, rollupBinEntrypoint)

	if !cometos.FileExists(entrypointBinaryFile) {
		if !cometos.FileExists(entrypointSourceFile) {
			return fmt.Errorf("%w: no entrypoint file: %s", ErrRunEntrypoint, entrypointSourceFile)
		}

		// try to build the entrypoint as a go binary
		buildArgs := []string{"build", "-o", entrypointBinaryFile, entrypointSourceFile}
		buildCmd := exec.Command("go", buildArgs...) //nolint:gosec
		buildCmd.Stdout = os.Stdout
		buildCmd.Stderr = os.Stderr
		if err := buildCmd.Run(); err != nil {
			return fmt.Errorf("%w: failed to build entrypoint: %w", ErrRunEntrypoint, err)
		}
	}

	var runArgs []string
	runArgs = append(runArgs, args...)
	if rollkitConfig.Chain.ConfigDir != "" {
		// The entrypoint is a separate binary based on https://github.com/rollkit/cosmos-sdk, so
		// we have to pass --home flag to the entrypoint to read the correct chain configuration files if specified.
		runArgs = append(runArgs, "--home", rollkitConfig.Chain.ConfigDir)
	}

	entrypointCmd := exec.Command(entrypointBinaryFile, runArgs...) //nolint:gosec
	entrypointCmd.Stdout = os.Stdout
	entrypointCmd.Stderr = os.Stderr

	if err := entrypointCmd.Run(); err != nil {
		return fmt.Errorf("%w: failed to run entrypoint: %w", ErrRunEntrypoint, err)
	}

	return nil
}
