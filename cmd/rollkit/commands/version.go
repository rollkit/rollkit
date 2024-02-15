package commands

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// GitSHA is set at build time
var GitSHA string

// Version is set at build time
var Version string

// VersionCmd ...
var VersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version info",
	RunE: func(cmd *cobra.Command, args []string) error {
		if GitSHA == "" {
			return fmt.Errorf("version not set")
		}
		if Version == "" {
			return fmt.Errorf("version not set")
		}
		w := tabwriter.NewWriter(os.Stdout, 2, 0, 2, ' ', 0)
		fmt.Fprintf(w, "\nrollkit version:\t%v\n", Version)
		fmt.Fprintf(w, "rollkit git sha:\t%v\n", GitSHA)
		fmt.Fprintln(w, "")
		return w.Flush()
	},
}
