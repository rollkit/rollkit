package commands

import (
	"errors"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	rollconf "github.com/rollkit/rollkit/config"
)

type mockDeps struct {
	mockReadToml      func() (rollconf.NodeConfig, error)
	mockRunEntrypoint func(rollkitConfig *rollconf.NodeConfig, args []string) error
}

func TestInterceptCommand(t *testing.T) {
	t.Run("intercepts command and runs entrypoint", func(t *testing.T) {
		deps := mockDeps{
			mockReadToml: func() (rollconf.NodeConfig, error) {
				return rollconf.NodeConfig{
					RootDir:    "/test",
					Entrypoint: "/test/main.go",
					Chain:      rollconf.ChainConfig{ConfigDir: "/test/config"},
				}, nil
			},
			mockRunEntrypoint: func(config *rollconf.NodeConfig, flags []string) error {
				return nil
			},
		}

		os.Args = []string{"rollkit", "start"}
		cmd := &cobra.Command{Use: "test"}
		cmd.AddCommand(&cobra.Command{Use: "docs-gen"})
		cmd.AddCommand(&cobra.Command{Use: "toml"})

		ok, err := InterceptCommand(cmd, deps.mockReadToml, deps.mockRunEntrypoint)
		require.NoError(t, err)
		require.True(t, ok)
	})

	t.Run("intercepts command and runs entrypoint with different root dir", func(t *testing.T) {
		deps := mockDeps{
			mockReadToml: func() (rollconf.NodeConfig, error) {
				return rollconf.NodeConfig{
					RootDir:    "/central",
					Entrypoint: "/central/main.go",
					Chain:      rollconf.ChainConfig{ConfigDir: "/central/config"},
				}, nil
			},
			mockRunEntrypoint: func(config *rollconf.NodeConfig, flags []string) error {
				return nil
			},
		}

		os.Args = []string{"rollkit", "start", "--rollkit.da_address=http://centralized-da:26657"}
		cmd := &cobra.Command{Use: "test"}

		ok, err := InterceptCommand(cmd, deps.mockReadToml, deps.mockRunEntrypoint)
		require.NoError(t, err)
		require.True(t, ok)
	})

	t.Run("returns error if readToml fails", func(t *testing.T) {
		deps := mockDeps{
			mockReadToml: func() (rollconf.NodeConfig, error) {
				return rollconf.NodeConfig{}, errors.New("read error")
			},
		}

		os.Args = []string{"rollkit", "start"}
		cmd := &cobra.Command{Use: "test"}

		_, err := InterceptCommand(cmd, deps.mockReadToml, nil)
		require.Error(t, err)
	})

	t.Run("returns error if entrypoint is empty", func(t *testing.T) {
		deps := mockDeps{
			mockReadToml: func() (rollconf.NodeConfig, error) {
				return rollconf.NodeConfig{Entrypoint: ""}, nil
			},
		}

		os.Args = []string{"rollkit", "start"}
		cmd := &cobra.Command{Use: "test"}

		_, err := InterceptCommand(cmd, deps.mockReadToml, nil)
		require.Error(t, err)
	})

	t.Run("does not intercept help command", func(t *testing.T) {
		deps := mockDeps{
			mockReadToml: func() (rollconf.NodeConfig, error) {
				return rollconf.NodeConfig{
					RootDir:    "/test",
					Entrypoint: "/test/main.go",
					Chain:      rollconf.ChainConfig{ConfigDir: "/test/config"},
				}, nil
			},
			mockRunEntrypoint: func(config *rollconf.NodeConfig, flags []string) error {
				return nil
			},
		}

		os.Args = []string{"rollkit", "-h"}
		cmd := &cobra.Command{Use: "test"}

		ok, err := InterceptCommand(cmd, deps.mockReadToml, deps.mockRunEntrypoint)
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("does not intercept version command", func(t *testing.T) {
		deps := mockDeps{
			mockReadToml: func() (rollconf.NodeConfig, error) {
				return rollconf.NodeConfig{
					RootDir:    "/test",
					Entrypoint: "/test/main.go",
					Chain:      rollconf.ChainConfig{ConfigDir: "/test/config"},
				}, nil
			},
			mockRunEntrypoint: func(config *rollconf.NodeConfig, flags []string) error {
				return nil
			},
		}

		os.Args = []string{"rollkit", "--version"}
		cmd := &cobra.Command{Use: "test"}

		ok, err := InterceptCommand(cmd, deps.mockReadToml, deps.mockRunEntrypoint)
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("does not intercept rollkit command", func(t *testing.T) {
		deps := mockDeps{
			mockReadToml: func() (rollconf.NodeConfig, error) {
				return rollconf.NodeConfig{
					RootDir:    "/test",
					Entrypoint: "/test/main.go",
					Chain:      rollconf.ChainConfig{ConfigDir: "/test/config"},
				}, nil
			},
			mockRunEntrypoint: func(config *rollconf.NodeConfig, flags []string) error {
				return nil
			},
		}

		os.Args = []string{"rollkit", "docs-gen"}
		cmd := &cobra.Command{Use: "test"}
		cmd.AddCommand(&cobra.Command{Use: "docs-gen"})

		ok, err := InterceptCommand(cmd, deps.mockReadToml, deps.mockRunEntrypoint)
		require.NoError(t, err)
		require.False(t, ok)
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
