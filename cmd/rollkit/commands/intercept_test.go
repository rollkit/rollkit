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
		wantExecuted      bool
	}{
		{
			name:            "Successful intercept with entrypoint",
			rollkitCommands: []string{"docs-gen", "toml"},
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
			args:         []string{"rollkit", "start"},
			wantErr:      false,
			wantExecuted: true,
		},
		{
			name:            "Intercept with centralized service flags",
			rollkitCommands: []string{"docs-gen", "toml"},
			mockReadToml: func() (rollconf.TomlConfig, error) {
				return rollconf.TomlConfig{
					Entrypoint: "centralized-entrypoint",
					Chain:      rollconf.ChainTomlConfig{ConfigDir: "/central/config"},
					RootDir:    "/central/root",
				}, nil
			},
			mockRunEntrypoint: func(config *rollconf.TomlConfig, flags []string) error {
				// Assert that the flags include the centralized addresses
				if parseFlag(flags, rollconf.FlagDAAddress) != "http://centralized-da:26657" {
					return errors.New("centralized DA address not passed")
				}
				if parseFlag(flags, rollconf.FlagSequencerAddress) != "centralized-seq:26659" {
					return errors.New("centralized Sequencer address not passed")
				}
				if parseFlag(flags, rollconf.FlagSequencerRollupID) != "centralizedrollup" {
					return errors.New("centralized rollup id not passed")
				}
				return nil
			},
			args:         []string{"rollkit", "start", "--rollkit.da_address=http://centralized-da:26657", "--rollkit.sequencer_address=centralized-seq:26659", "--rollkit.sequencer_rollup_id=centralizedrollup"},
			wantErr:      false,
			wantExecuted: true,
		},
		{
			name:            "Configuration read error",
			rollkitCommands: []string{"docs-gen", "toml"},
			mockReadToml: func() (rollconf.TomlConfig, error) {
				return rollconf.TomlConfig{}, errors.New("read error")
			},
			args:         []string{"rollkit", "start"},
			wantErr:      true,
			wantExecuted: false,
		},
		{
			name:            "Empty entrypoint",
			rollkitCommands: []string{"docs-gen", "toml"},
			mockReadToml: func() (rollconf.TomlConfig, error) {
				return rollconf.TomlConfig{Entrypoint: ""}, nil
			},
			args:         []string{"rollkit", "start"},
			wantErr:      true,
			wantExecuted: true,
		},
		{
			name:            "Skip intercept, rollkit command",
			rollkitCommands: []string{"docs-gen", "toml"},
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
			args:         []string{"rollkit", "docs-gen"},
			wantErr:      false,
			wantExecuted: false,
		},
		{
			name:            "Skip intercept, help command",
			rollkitCommands: []string{"docs-gen", "toml"},
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
			args:         []string{"rollkit", "-h"},
			wantErr:      false,
			wantExecuted: false,
		},
		{
			name:            "Skip intercept, rollkit repository itself",
			rollkitCommands: []string{"docs-gen", "toml"},
			mockReadToml: func() (rollconf.TomlConfig, error) {
				return rollconf.TomlConfig{
					Entrypoint: "test-entrypoint",
					Chain:      rollconf.ChainTomlConfig{ConfigDir: "/test/config"},

					RootDir: "/test/rollkit",
				}, nil
			},
			mockRunEntrypoint: func(config *rollconf.TomlConfig, flags []string) error {
				return nil
			},
			args:         []string{"rollkit", "start"},
			wantErr:      false,
			wantExecuted: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Args = tt.args

			cmd := &cobra.Command{Use: "test"}
			for _, c := range tt.rollkitCommands {
				cmd.AddCommand(&cobra.Command{Use: c})
			}

			ok, err := InterceptCommand(
				cmd,
				tt.mockReadToml,
				tt.mockRunEntrypoint,
			)
			if (err != nil) != tt.wantErr {
				t.Errorf("InterceptCommand() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if ok != tt.wantExecuted {
				t.Errorf("InterceptCommand() executed = %v, wantExecuted %v", ok, tt.wantExecuted)
				return
			}
		})
	}

}
