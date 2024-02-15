package commands

import (
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// DocsGenCmd is the command to generate documentation for rollkit CLI
var DocsGenCmd = &cobra.Command{
	Use:   "docs-gen",
	Short: "Generate documentation for rollkit CLI",
	RunE: func(cmd *cobra.Command, args []string) error {
		return doc.GenMarkdownTree(RootCmd, "./cmd/rollkit/docs")
	},
}
