package config

import (
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

func TestDefaultNodeConfig(t *testing.T) {
	// Test that default config has expected values
	def := DefaultNodeConfig

	assert.Equal(t, DefaultRootDir(), def.RootDir)
	assert.Equal(t, "data", def.DBPath)
	assert.Equal(t, false, def.Rollkit.Aggregator)
	assert.Equal(t, false, def.Rollkit.Light)
	assert.Equal(t, DefaultDAAddress, def.Rollkit.DAAddress)
	assert.Equal(t, "", def.Rollkit.DAAuthToken)
	assert.Equal(t, float64(-1), def.Rollkit.DAGasPrice)
	assert.Equal(t, float64(0), def.Rollkit.DAGasMultiplier)
	assert.Equal(t, "", def.Rollkit.DASubmitOptions)
	assert.Equal(t, "", def.Rollkit.DANamespace)
	assert.Equal(t, 1*time.Second, def.Rollkit.BlockTime)
	assert.Equal(t, 15*time.Second, def.Rollkit.DABlockTime)
	assert.Equal(t, uint64(0), def.Rollkit.DAStartHeight)
	assert.Equal(t, uint64(0), def.Rollkit.DAMempoolTTL)
	assert.Equal(t, uint64(0), def.Rollkit.MaxPendingBlocks)
	assert.Equal(t, false, def.Rollkit.LazyAggregator)
	assert.Equal(t, 60*time.Second, def.Rollkit.LazyBlockTime)
	assert.Equal(t, "", def.Rollkit.TrustedHash)
	assert.Equal(t, DefaultSequencerAddress, def.Rollkit.SequencerAddress)
	assert.Equal(t, DefaultSequencerRollupID, def.Rollkit.SequencerRollupID)
	assert.Equal(t, DefaultExecutorAddress, def.Rollkit.ExecutorAddress)
}

func TestAddFlags(t *testing.T) {
	cmd := &cobra.Command{}
	AddFlags(cmd)

	// Test that all flags are added
	flags := cmd.Flags()

	// Test root flags
	assert.NotNil(t, flags.Lookup(FlagRootDir))
	assert.NotNil(t, flags.Lookup(FlagDBPath))

	// Test Rollkit flags
	assert.NotNil(t, flags.Lookup(FlagAggregator))
	assert.NotNil(t, flags.Lookup(FlagLazyAggregator))
	assert.NotNil(t, flags.Lookup(FlagDAAddress))
	assert.NotNil(t, flags.Lookup(FlagDAAuthToken))
	assert.NotNil(t, flags.Lookup(FlagBlockTime))
	assert.NotNil(t, flags.Lookup(FlagDABlockTime))
	assert.NotNil(t, flags.Lookup(FlagDAGasPrice))
	assert.NotNil(t, flags.Lookup(FlagDAGasMultiplier))
	assert.NotNil(t, flags.Lookup(FlagDAStartHeight))
	assert.NotNil(t, flags.Lookup(FlagDANamespace))
	assert.NotNil(t, flags.Lookup(FlagDASubmitOptions))
	assert.NotNil(t, flags.Lookup(FlagLight))
	assert.NotNil(t, flags.Lookup(FlagTrustedHash))
	assert.NotNil(t, flags.Lookup(FlagMaxPendingBlocks))
	assert.NotNil(t, flags.Lookup(FlagDAMempoolTTL))
	assert.NotNil(t, flags.Lookup(FlagLazyBlockTime))
	assert.NotNil(t, flags.Lookup(FlagSequencerAddress))
	assert.NotNil(t, flags.Lookup(FlagSequencerRollupID))
	assert.NotNil(t, flags.Lookup(FlagExecutorAddress))

	// Test instrumentation flags
	assert.NotNil(t, flags.Lookup(FlagPrometheus))
	assert.NotNil(t, flags.Lookup(FlagPrometheusListenAddr))
	assert.NotNil(t, flags.Lookup(FlagMaxOpenConnections))

	// Test P2P flags
	assert.NotNil(t, flags.Lookup(FlagP2PListenAddress))
	assert.NotNil(t, flags.Lookup(FlagP2PSeeds))
	assert.NotNil(t, flags.Lookup(FlagP2PBlockedPeers))
	assert.NotNil(t, flags.Lookup(FlagP2PAllowedPeers))

	// Verify that there are no additional flags
	// Count the number of flags we're explicitly checking
	expectedFlagCount := 28 // Update this number if you add more flag checks above

	// Get the actual number of flags
	actualFlagCount := 0
	flags.VisitAll(func(flag *pflag.Flag) {
		actualFlagCount++
	})

	// Verify that the counts match
	assert.Equal(t, expectedFlagCount, actualFlagCount, "Number of flags doesn't match. If you added a new flag, please update the test.")
}
