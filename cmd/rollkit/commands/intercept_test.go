package commands

import (
	"errors"
	"os"
	"testing"

	rollconf "github.com/rollkit/rollkit/config"

	"github.com/spf13/cobra"
)

func TestInterceptCommand(t *testing.T) {
	tests := []struct {
		name              string
		rollkitCommands   []string
		mockReadToml      func() (rollconf.TomlConfig, error)
		mockRunEntrypoint func(rollkitConfig *rollconf.TomlConfig, args []string) error
		args              []string
		wantErr           bool
	}{
		{
			name: "Successful intercept with entrypoint",
			mockReadToml: func() (rollconf.TomlConfig, error) {
				return rollconf.TomlConfig{
					Entrypoint: "test-entrypoint",
					Chain:      rollconf.ChainTomlConfig{ConfigDir: "/test/config"},

					RootDir: "/test/root",
				}, nil
			},
			mockRunEntrypoint: func(config *rollconf.TomlConfig, flags []string) error {
				return nil
			},
			args:    []string{"cmd", "arg1", "arg2"},
			wantErr: false,
		},
		{
			name: "Configuration read error",
			mockReadToml: func() (rollconf.TomlConfig, error) {
				return rollconf.TomlConfig{}, errors.New("read error")
			},
			args:    []string{"cmd"},
			wantErr: true,
		},
		{
			name: "Empty entrypoint",
			mockReadToml: func() (rollconf.TomlConfig, error) {
				return rollconf.TomlConfig{Entrypoint: ""}, nil
			},
			args:    []string{"cmd"},
			wantErr: true,
		},
		{
			name:            "Skip intercept",
			rollkitCommands: []string{"cmd1, cmd2"},
			mockReadToml: func() (rollconf.TomlConfig, error) {
				return rollconf.TomlConfig{
					Entrypoint: "test-entrypoint",
					Chain:      rollconf.ChainTomlConfig{ConfigDir: "/test/config"},

					RootDir: "/test/root",
				}, nil
			},
			mockRunEntrypoint: func(config *rollconf.TomlConfig, flags []string) error {
				return nil
			},
			args:    []string{"cmd"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Args = tt.args

			cmd := &cobra.Command{Use: "test"}
			for _, c := range tt.rollkitCommands {
				cmd.AddCommand(&cobra.Command{Use: c})
			}

			err := InterceptCommand(
				cmd,
				tt.mockReadToml,
				tt.mockRunEntrypoint,
			)
			if (err != nil) != tt.wantErr {
				t.Errorf("InterceptCommand() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}

}
