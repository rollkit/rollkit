package commands

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	rollconf "github.com/rollkit/rollkit/config"
)

type mockDeps struct {
	mockReadConfig    func() (rollconf.Config, error)
	mockRunEntrypoint func(rollkitConfig *rollconf.Config, args []string) error
}

func TestInterceptCommand(t *testing.T) {
	t.Run("intercepts command and runs entrypoint", func(t *testing.T) {
		// Create a temporary directory and file for the test
		tempDir := t.TempDir()
		entrypointPath := filepath.Join(tempDir, "main.go")
		err := os.WriteFile(entrypointPath, []byte("package main\nfunc main() {}\n"), 0600)
		require.NoError(t, err)

		deps := mockDeps{
			mockReadConfig: func() (rollconf.Config, error) {
				return rollconf.Config{
					RootDir:    tempDir,
					Entrypoint: "main.go",
					ConfigDir:  filepath.Join(tempDir, "config"),
				}, nil
			},
			mockRunEntrypoint: func(config *rollconf.Config, flags []string) error {
				return nil
			},
		}

		os.Args = []string{"rollkit", "start"}
		cmd := &cobra.Command{Use: "test"}
		cmd.AddCommand(&cobra.Command{Use: "docs-gen"})
		cmd.AddCommand(&cobra.Command{Use: "yaml"})

		ok, err := InterceptCommand(cmd, deps.mockReadConfig, deps.mockRunEntrypoint)
		require.NoError(t, err)
		require.True(t, ok)
	})

	t.Run("intercepts command and runs entrypoint with different root dir", func(t *testing.T) {
		// Create a temporary directory and file for the test
		tempDir := t.TempDir()
		entrypointPath := filepath.Join(tempDir, "main.go")
		err := os.WriteFile(entrypointPath, []byte("package main\nfunc main() {}\n"), 0600)
		require.NoError(t, err)

		deps := mockDeps{
			mockReadConfig: func() (rollconf.Config, error) {
				return rollconf.Config{
					RootDir:    tempDir,
					Entrypoint: "main.go",
					ConfigDir:  filepath.Join(tempDir, "config"),
				}, nil
			},
			mockRunEntrypoint: func(config *rollconf.Config, flags []string) error {
				return nil
			},
		}

		os.Args = []string{"rollkit", "start", "--rollkit.da_address=http://centralized-da:26657"}
		cmd := &cobra.Command{Use: "test"}

		ok, err := InterceptCommand(cmd, deps.mockReadConfig, deps.mockRunEntrypoint)
		require.NoError(t, err)
		require.True(t, ok)
	})

	t.Run("returns error if readConfig fails", func(t *testing.T) {
		deps := mockDeps{
			mockReadConfig: func() (rollconf.Config, error) {
				return rollconf.Config{}, errors.New("read error")
			},
		}

		os.Args = []string{"rollkit", "start"}
		cmd := &cobra.Command{Use: "test"}

		_, err := InterceptCommand(cmd, deps.mockReadConfig, nil)
		require.Error(t, err)
	})

	t.Run("returns error if entrypoint is empty", func(t *testing.T) {
		deps := mockDeps{
			mockReadConfig: func() (rollconf.Config, error) {
				return rollconf.Config{Entrypoint: ""}, nil
			},
		}

		os.Args = []string{"rollkit", "start"}
		cmd := &cobra.Command{Use: "test"}

		_, err := InterceptCommand(cmd, deps.mockReadConfig, nil)
		require.Error(t, err)
	})

	t.Run("does not intercept help command", func(t *testing.T) {
		deps := mockDeps{
			mockReadConfig: func() (rollconf.Config, error) {
				return rollconf.Config{
					RootDir:    "/test",
					Entrypoint: "/test/main.go",
					ConfigDir:  "/test/config",
				}, nil
			},
			mockRunEntrypoint: func(config *rollconf.Config, flags []string) error {
				return nil
			},
		}

		os.Args = []string{"rollkit", "-h"}
		cmd := &cobra.Command{Use: "test"}

		ok, err := InterceptCommand(cmd, deps.mockReadConfig, deps.mockRunEntrypoint)
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("does not intercept version command", func(t *testing.T) {
		deps := mockDeps{
			mockReadConfig: func() (rollconf.Config, error) {
				return rollconf.Config{
					RootDir:    "/test",
					Entrypoint: "/test/main.go",
					ConfigDir:  "/test/config",
				}, nil
			},
			mockRunEntrypoint: func(config *rollconf.Config, flags []string) error {
				return nil
			},
		}

		os.Args = []string{"rollkit", "--version"}
		cmd := &cobra.Command{Use: "test"}

		ok, err := InterceptCommand(cmd, deps.mockReadConfig, deps.mockRunEntrypoint)
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("does not intercept rollkit command", func(t *testing.T) {
		deps := mockDeps{
			mockReadConfig: func() (rollconf.Config, error) {
				return rollconf.Config{
					RootDir:    "/test",
					Entrypoint: "/test/main.go",
					ConfigDir:  "/test/config",
				}, nil
			},
			mockRunEntrypoint: func(config *rollconf.Config, flags []string) error {
				return nil
			},
		}

		os.Args = []string{"rollkit", "docs-gen"}
		cmd := &cobra.Command{Use: "test"}
		cmd.AddCommand(&cobra.Command{Use: "docs-gen"})

		ok, err := InterceptCommand(cmd, deps.mockReadConfig, deps.mockRunEntrypoint)
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("does not intercept command when entrypoint is empty", func(t *testing.T) {
		deps := mockDeps{
			mockReadConfig: func() (rollconf.Config, error) {
				return rollconf.Config{
					RootDir:    "/central",
					Entrypoint: "",
					ConfigDir:  "/central/config",
				}, nil
			},
			mockRunEntrypoint: func(config *rollconf.Config, flags []string) error {
				return nil
			},
		}

		os.Args = []string{"rollkit", "start"}
		cmd := &cobra.Command{Use: "test"}

		_, err := InterceptCommand(cmd, deps.mockReadConfig, deps.mockRunEntrypoint)
		require.Error(t, err)
	})

	t.Run("does not intercept command when entrypoint is not found", func(t *testing.T) {
		deps := mockDeps{
			mockReadConfig: func() (rollconf.Config, error) {
				return rollconf.Config{
					RootDir:    "/test",
					Entrypoint: "/test/not-found.go",
					ConfigDir:  "/test/config",
				}, nil
			},
			mockRunEntrypoint: func(config *rollconf.Config, flags []string) error {
				return nil
			},
		}

		os.Args = []string{"rollkit", "start"}
		cmd := &cobra.Command{Use: "test"}

		_, err := InterceptCommand(cmd, deps.mockReadConfig, deps.mockRunEntrypoint)
		require.Error(t, err)
	})

	t.Run("does not intercept command when entrypoint is not a go file", func(t *testing.T) {
		deps := mockDeps{
			mockReadConfig: func() (rollconf.Config, error) {
				return rollconf.Config{
					RootDir:    "/test",
					Entrypoint: "/test/not-a-go-file",
					ConfigDir:  "/test/config",
				}, nil
			},
			mockRunEntrypoint: func(config *rollconf.Config, flags []string) error {
				return nil
			},
		}

		os.Args = []string{"rollkit", "start"}
		cmd := &cobra.Command{Use: "test"}

		_, err := InterceptCommand(cmd, deps.mockReadConfig, deps.mockRunEntrypoint)
		require.Error(t, err)
	})

	t.Run("does not intercept command when entrypoint is a directory", func(t *testing.T) {
		deps := mockDeps{
			mockReadConfig: func() (rollconf.Config, error) {
				return rollconf.Config{
					RootDir:    "/test",
					Entrypoint: "/test",
					ConfigDir:  "/test/config",
				}, nil
			},
			mockRunEntrypoint: func(config *rollconf.Config, flags []string) error {
				return nil
			},
		}

		os.Args = []string{"rollkit", "start"}
		cmd := &cobra.Command{Use: "test"}

		_, err := InterceptCommand(cmd, deps.mockReadConfig, deps.mockRunEntrypoint)
		require.Error(t, err)
	})
}

// TestParseFlagWithEquals tests the parseFlag function with different flag formats.
func TestParseFlagWithEquals(t *testing.T) {
	testCases := []struct {
		name     string
		args     []string
		flag     string
		expected string
	}{
		{
			name:     "Equals style simple",
			args:     []string{"--rollkit.myflag=value"},
			flag:     "rollkit.myflag",
			expected: "value",
		},
		{
			name:     "Equals style complex",
			args:     []string{"--rollkit.myflag=some=complex=value"},
			flag:     "rollkit.myflag",
			expected: "some=complex=value",
		},
		{
			name:     "Space separated",
			args:     []string{"--rollkit.myflag", "value"},
			flag:     "rollkit.myflag",
			expected: "value",
		},
		{
			name:     "Flag not present",
			args:     []string{"--rollkit.otherflag=123"},
			flag:     "rollkit.myflag",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseFlag(tc.args, tc.flag)
			if got != tc.expected {
				t.Errorf("parseFlag(%v, %q) = %q; expected %q", tc.args, tc.flag, got, tc.expected)
			}
		})
	}
}
