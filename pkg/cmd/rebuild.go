package cmd

import (
	"log"

	"github.com/spf13/cobra"

	rollconf "github.com/rollkit/rollkit/pkg/config"
)

// RebuildCmd is a command to rebuild rollup entrypoint
var RebuildCmd = &cobra.Command{
	Use:   "rebuild",
	Short: "Rebuild rollup entrypoint",
	Long:  "Rebuild rollup entrypoint specified in the rollkit.yaml",
	Run: func(cmd *cobra.Command, args []string) {
		var err error
		rollkitConfig, err = rollconf.ReadYaml("")
		if err != nil {
			log.Fatalf("Could not read rollkit.yaml file: %s", err)
		}

		if _, err := buildEntrypoint(rollkitConfig.RootDir, rollkitConfig.Entrypoint, true); err != nil {
			log.Fatalf("Could not rebuild rollup entrypoint: %s", err)
		}
	},
}
