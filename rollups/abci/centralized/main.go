package centralized

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cometbft/cometbft/libs/cli"

	rollconf "github.com/rollkit/rollkit/config"
	cmd "github.com/rollkit/rollkit/rollups/abci/centralized/commands"
)

func main() {
	// Initiate the root command
	rootCmd := cmd.RootCmd

	// Add subcommands to the root command
	rootCmd.AddCommand(
		cmd.NewRunNodeCmd(),
		cmd.VersionCmd,
		cmd.NewTomlCmd(),
		cmd.RebuildCmd,
	)

	// In case there is a rollkit.toml file in the current dir or somewhere up the
	// directory tree - we want to intercept the command and execute it against an entrypoint
	// specified in the rollkit.toml file. In case of missing toml file or missing entrypoint key
	// or missing actual entrypoint file - the normal rootCmd command is executed.
	executed, err := cmd.InterceptCommand(
		rootCmd,
		rollconf.ReadToml,
		cmd.RunRollupEntrypoint,
	)
	if err != nil {
		fmt.Println("Error intercepting command: ", err)
	}
	if executed {
		return
	}

	// Prepare the base command and execute
	executor := cli.PrepareBaseCmd(rootCmd, "RK", os.ExpandEnv(filepath.Join("$HOME", ".centralized-abci")))
	if err := executor.Execute(); err != nil {
		// Print to stderr and exit with error
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
