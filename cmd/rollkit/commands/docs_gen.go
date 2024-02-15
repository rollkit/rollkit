package commands

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// DocsGenCmd ...
var DocsGenCmd = &cobra.Command{
	Use:   "docs-gen",
	Short: "Generate documentation for rollkit CLI",
	RunE: func(cmd *cobra.Command, args []string) error {
		if GitSHA == "" {
			return errors.New("version not set")
		}
		if Version == "" {
			return errors.New("version not set")
		}
		return doc.GenMarkdownTree(RootCmd, "./cmd/rollkit/docs")
	},
}
