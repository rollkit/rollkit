package commands

import (
	"fmt"
	"os"

	rollconf "github.com/rollkit/rollkit/config"

	"github.com/spf13/cobra"
)

// RebuildCmd is a command to rebuild rollup entrypoint
var RebuildCmd = &cobra.Command{
	Use:   "rebuild",
	Short: "Rebuild rollup entrypoint",
	Long:  "Rebuild rollup entrypoint specified in the rollkit.toml",
	Run: func(cmd *cobra.Command, args []string) {
		var err error
		rollkitConfig, err = rollconf.ReadToml()
		if err != nil {
			log.Fatalf("Could not read rollkit.toml file: %s", err)
		}

		if _, err := buildEntrypoint(rollkitConfig.RootDir, rollkitConfig.Entrypoint, true); err != nil {
			log.Fatalf("Could not rebuild rollup entrypoint: %s", err)
		}
	},
}
