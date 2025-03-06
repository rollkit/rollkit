package config

import (
	"testing"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// TestNodeConfigIntegration is a unified test that tests all functionalities
// related to NodeConfig configuration, including reading from Viper,
// integration with Cobra, and setting all flags.
func TestNodeConfigIntegration(t *testing.T) {
	// Subtest to test reading basic configurations
	t.Run("BasicConfigFields", func(t *testing.T) {
		// Test RootDir
		t.Run("RootDir", func(t *testing.T) {
			v := viper.New()
			v.Set("home", "~/custom-root")

			nc := NodeConfig{}
			err := v.Unmarshal(&nc)
			assert.NoError(t, err)

			assert.Equal(t, "~/custom-root", nc.RootDir)
		})

		// Test DBPath
		t.Run("DBPath", func(t *testing.T) {
			v := viper.New()
			v.Set("rollkit.db_path", "./custom-db")

			nc := NodeConfig{}
			err := v.Unmarshal(&nc)
			assert.NoError(t, err)

			assert.Equal(t, "./custom-db", nc.Rollkit.DBPath)
		})
	})

	// Subtest to test integration with Cobra
	t.Run("CobraIntegration", func(t *testing.T) {
		cmd := &cobra.Command{}
		AddFlags(cmd)

		v := viper.New()
		assert.NoError(t, v.BindPFlags(cmd.Flags()))

		// Configure some flags
		assert.NoError(t, cmd.Flags().Set(FlagAggregator, "true"))
		assert.NoError(t, cmd.Flags().Set(FlagDAAddress, "http://da.example.com"))
		assert.NoError(t, cmd.Flags().Set(FlagBlockTime, "5s"))

		// Start with a clean config
		nc := NodeConfig{
			Rollkit: RollkitConfig{},
		}
		err := v.Unmarshal(&nc, func(c *mapstructure.DecoderConfig) {
			c.TagName = "mapstructure"
			c.DecodeHook = mapstructure.ComposeDecodeHookFunc(
				mapstructure.StringToTimeDurationHookFunc(),
				mapstructure.StringToSliceHookFunc(","),
			)
		})
		assert.NoError(t, err)

		// Ensure Instrumentation namespace is set
		if nc.Instrumentation != nil {
			nc.Instrumentation.Namespace = "rollkit"
		}

		// Verify that the values have been set correctly
		assert.Equal(t, true, nc.Rollkit.Aggregator)
		assert.Equal(t, "http://da.example.com", nc.Rollkit.DAAddress)
		assert.Equal(t, 5*time.Second, nc.Rollkit.BlockTime)
	})
}
