package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	cometos "github.com/cometbft/cometbft/libs/os"
	"github.com/spf13/cobra"

	rollconf "github.com/rollkit/rollkit/config"
)

const rollupBinEntrypoint = "entrypoint"

var rollkitConfig rollconf.Config

// InterceptCommand intercepts the command and runs it against the `entrypoint`
// specified in the rollkit.toml configuration file.
func InterceptCommand(
	rollkitCommand *cobra.Command,
	readToml func() (rollconf.Config, error),
	runEntrypoint func(*rollconf.Config, []string) error,
) (shouldExecute bool, err error) {
	// Grab flags and verify command
	flags := []string{}
	if len(os.Args) >= 2 {
		flags = os.Args[1:]

		// Handle specific cases first for help, version, and start
		switch os.Args[1] {
		case "help", "--help", "h", "-h",
			"version", "--version", "v", "-v":
			return
		default:
			// Check if user attempted to run a rollkit command
			for _, cmd := range rollkitCommand.Commands() {
				if os.Args[1] == cmd.Use {
					return
				}
			}
		}
	}

	rollkitConfig, err = readToml()
	if err != nil {
		return
	}

	// To avoid recursive calls, we check if the root directory is the rollkit repository itself
	if filepath.Base(rollkitConfig.RootDir) == "rollkit" {
		return
	}

	// At this point we expect to execute the command against the entrypoint
	shouldExecute = true

	// After successfully reading the TOML file, we expect to be able to use the entrypoint
	if rollkitConfig.Entrypoint == "" {
		err = fmt.Errorf("no entrypoint specified in %s", rollconf.RollkitConfigToml)
		return
	}

	return shouldExecute, runEntrypoint(&rollkitConfig, flags)
}

func buildEntrypoint(rootDir, entrypointSourceFile string, forceRebuild bool) (string, error) {
	// The entrypoint binary file is always in the same directory as the rollkit.toml file.
	entrypointBinaryFile := filepath.Join(rootDir, rollupBinEntrypoint)

	if !cometos.FileExists(entrypointBinaryFile) || forceRebuild {
		if !cometos.FileExists(entrypointSourceFile) {
			return "", fmt.Errorf("no entrypoint source file: %s", entrypointSourceFile)
		}

		// try to build the entrypoint as a go binary
		buildArgs := []string{"build", "-o", entrypointBinaryFile, entrypointSourceFile}
		buildCmd := exec.Command("go", buildArgs...) //nolint:gosec
		buildCmd.Stdout = os.Stdout
		buildCmd.Stderr = os.Stderr
		if err := buildCmd.Run(); err != nil {
			return "", fmt.Errorf("failed to build entrypoint: %w", err)
		}
	}

	return entrypointBinaryFile, nil
}

// RunRollupEntrypoint runs the entrypoint specified in the rollkit.toml configuration file.
// If the entrypoint is not built, it will build it first. The entrypoint is built
// in the same directory as the rollkit.toml file. The entrypoint is run with the
// same flags as the original command, but with the `--home` flag set to the config
// directory of the chain specified in the rollkit.toml file. This is so the entrypoint,
// which is a separate binary of the rollup, can read the correct chain configuration files.
func RunRollupEntrypoint(rollkitConfig *rollconf.Config, args []string) error {
	var entrypointSourceFile string
	if !filepath.IsAbs(rollkitConfig.RootDir) {
		entrypointSourceFile = filepath.Join(rollkitConfig.RootDir, rollkitConfig.Entrypoint)
	} else {
		entrypointSourceFile = rollkitConfig.Entrypoint
	}

	entrypointBinaryFilePath, err := buildEntrypoint(rollkitConfig.RootDir, entrypointSourceFile, false)
	if err != nil {
		return err
	}

	var runArgs []string
	runArgs = append(runArgs, args...)
	if rollkitConfig.Chain.ConfigDir != "" {
		// The entrypoint is a separate binary based on https://github.com/rollkit/cosmos-sdk, so
		// we have to pass --home flag to the entrypoint to read the correct chain configuration files if specified.
		runArgs = append(runArgs, "--home", rollkitConfig.Chain.ConfigDir)
	}

	entrypointCmd := exec.Command(entrypointBinaryFilePath, runArgs...) //nolint:gosec
	entrypointCmd.Stdout = os.Stdout
	entrypointCmd.Stderr = os.Stderr
	entrypointCmd.Stdin = os.Stdin

	if err := entrypointCmd.Run(); err != nil {
		return fmt.Errorf("failed to run entrypoint: %w", err)
	}

	return nil
}

func parseFlag(args []string, flag string) string {
	// Loop through all arguments to find the specified flag.
	// Supports both "--flag=value" and "--flag value" formats.
	for i, arg := range args {
		prefixEqual := fmt.Sprintf("--%s=", flag)
		prefix := fmt.Sprintf("--%s", flag)
		if strings.HasPrefix(arg, prefixEqual) {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) == 2 {
				return parts[1]
			}
		} else if arg == prefix {
			if i+1 < len(args) {
				return args[i+1]
			}
		}
	}
	return ""
}
