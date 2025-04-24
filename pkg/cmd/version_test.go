package cmd

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeCommand executes the given Cobra command with the provided args
// and captures its stdout/stderr.
func executeCommand(cmd *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return buf.String(), err
}

func TestVersionCmd_Success(t *testing.T) {
	Version = "v0.1.0-test"
	GitSHA = "abcdef123test"

	output, err := executeCommand(VersionCmd)

	require.NoError(t, err)
	assert.Contains(t, output, "v0.1.0-test")
	assert.Contains(t, output, "abcdef123test")
}

func TestVersionCmd_MissingVersion(t *testing.T) {
	Version = ""
	GitSHA = "abcdef123test"

	_, err := executeCommand(VersionCmd)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "version not set")
}

func TestVersionCmd_MissingGitSHA(t *testing.T) {
	GitSHA = ""
	Version = "v0.1.0-test"

	_, err := executeCommand(VersionCmd)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "git SHA not set")
}
