package commands

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

var docsDirectory = "./cmd/rollkit/docs"

// DocsGenCmd is the command to generate documentation for rollkit CLI
var DocsGenCmd = &cobra.Command{
	Use:   "docs-gen",
	Short: "Generate documentation for rollkit CLI",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Clear out the docs directory
		err := os.RemoveAll(docsDirectory)
		if err != nil {
			return err
		}
		// Initiate the docs directory
		err = os.MkdirAll(docsDirectory, os.ModePerm)
		if err != nil {
			return err
		}
		return doc.GenMarkdownTree(RootCmd, docsDirectory)
	},
}
