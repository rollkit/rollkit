package cmd

import (
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var (
	// GitSHA is set at build time
	GitSHA string

	// Version is set at build time
	Version string
)

// VersionCmd is the command to show version info for rollkit CLI
var VersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version info",
	RunE: func(cmd *cobra.Command, args []string) error {
		if GitSHA == "" {
			return errors.New("git SHA not set")
		}
		if Version == "" {
			return errors.New("version not set")
		}
		w := tabwriter.NewWriter(os.Stdout, 2, 0, 2, ' ', 0)
		_, err1 := fmt.Fprintf(w, "\nrollkit version:\t%v\n", Version)
		_, err2 := fmt.Fprintf(w, "rollkit git sha:\t%v\n", GitSHA)
		_, err3 := fmt.Fprintln(w, "")
		return errors.Join(err1, err2, err3, w.Flush())
	},
}
