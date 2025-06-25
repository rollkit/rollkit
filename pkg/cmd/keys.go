package cmd

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	rollconf "github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/signer/file"
)

// KeysCmd returns a command for managing keys.
func KeysCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "Manage signing keys",
	}

	cmd.AddCommand(exportKeyCmd())
	cmd.AddCommand(importKeyCmd())

	return cmd
}

func exportKeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export the private key to plain text",
		Long: `Export the locally saved signing key to plain text.
This allows the key to be stored in a password manager or other secure storage.

WARNING: The exported key is not encrypted. Handle it with extreme care.
Anyone with access to the exported key can sign messages on your behalf.
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeConfig, err := rollconf.Load(cmd)
			if err != nil {
				return fmt.Errorf("failed to load node config: %w", err)
			}

			passphrase, err := cmd.Flags().GetString(rollconf.FlagSignerPassphrase)
			if err != nil {
				return err
			}
			if passphrase == "" {
				return fmt.Errorf("passphrase is required. Please provide it using the --%s flag", rollconf.FlagSignerPassphrase)
			}

			keyPath := filepath.Join(nodeConfig.RootDir, "config")

			// Print a strong warning to stderr
			cmd.PrintErrln("WARNING: EXPORTING PRIVATE KEY. HANDLE WITH EXTREME CARE.")
			cmd.PrintErrln("ANYONE WITH ACCESS TO THIS KEY CAN SIGN MESSAGES ON YOUR BEHALF.")

			privKeyBytes, err := file.ExportPrivateKey(keyPath, []byte(passphrase))
			if err != nil {
				return fmt.Errorf("failed to export private key: %w", err)
			}

			hexKey := hex.EncodeToString(privKeyBytes)
			cmd.Println(hexKey)

			return nil
		},
	}
	cmd.Flags().String(rollconf.FlagSignerPassphrase, "", "Passphrase for the signer key")
	return cmd
}

func importKeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import [hex-private-key]",
		Short: "Import a private key from plain text",
		Long: `Import a decrypted private key and generate the encrypted file locally.
This allows the system to use it for signing.

The private key should be provided as a hex-encoded string.

WARNING: This command will overwrite any existing key.
If a 'signer.json' file exists in your home directory, you must use the --force flag.
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeConfig, err := rollconf.Load(cmd)
			if err != nil {
				return fmt.Errorf("failed to load node config: %w", err)
			}

			passphrase, err := cmd.Flags().GetString(rollconf.FlagSignerPassphrase)
			if err != nil {
				return err
			}
			if passphrase == "" {
				return fmt.Errorf("passphrase is required to encrypt the imported key. Please provide it using the --%s flag", rollconf.FlagSignerPassphrase)
			}

			hexKey := args[0]
			privKeyBytes, err := hex.DecodeString(hexKey)
			if err != nil {
				return fmt.Errorf("failed to decode hex private key: %w", err)
			}

			keyPath := filepath.Join(nodeConfig.RootDir, "config")
			filePath := filepath.Join(keyPath, "signer.json")

			// Check if file exists and if --force is used
			force, _ := cmd.Flags().GetBool("force")
			if _, err := os.Stat(filePath); err == nil && !force {
				return fmt.Errorf("key file already exists at %s. Use --force to overwrite", filePath)
			}

			if err := file.ImportPrivateKey(keyPath, privKeyBytes, []byte(passphrase)); err != nil {
				return fmt.Errorf("failed to import private key: %w", err)
			}

			cmd.Printf("Successfully imported key and saved to %s\n", filePath)
			return nil
		},
	}

	cmd.Flags().Bool("force", false, "Overwrite existing key file if it exists")
	cmd.Flags().String(rollconf.FlagSignerPassphrase, "", "Passphrase to encrypt the imported key")
	return cmd
}
