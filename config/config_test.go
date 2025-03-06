package config

import (
	"testing"
	"time"

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
		testCases := []struct {
			name     string
			viperSet map[string]interface{}
			validate func(t *testing.T, nc NodeConfig)
		}{
			{
				name: "RootDir",
				viperSet: map[string]interface{}{
					"root_dir": "~/custom-root",
				},
				validate: func(t *testing.T, nc NodeConfig) {
					assert.Equal(t, "~/custom-root", nc.RootDir)
				},
			},
			{
				name: "DBPath",
				viperSet: map[string]interface{}{
					FlagDBPath: "./custom-db",
				},
				validate: func(t *testing.T, nc NodeConfig) {
					assert.Equal(t, "./custom-db", nc.Rollkit.DBPath)
				},
			},
			{
				name: "P2P Configuration",
				viperSet: map[string]interface{}{
					"p2p.laddr":         "tcp://0.0.0.0:26656",
					"p2p.seeds":         "seed1,seed2",
					"p2p.blocked_peers": "peer1,peer2",
					"p2p.allowed_peers": "peer3,peer4",
				},
				validate: func(t *testing.T, nc NodeConfig) {
					assert.Equal(t, "tcp://0.0.0.0:26656", nc.P2P.ListenAddress)
					assert.Equal(t, "seed1,seed2", nc.P2P.Seeds)
					assert.Equal(t, "peer1,peer2", nc.P2P.BlockedPeers)
					assert.Equal(t, "peer3,peer4", nc.P2P.AllowedPeers)
				},
			},
			{
				name: "Instrumentation",
				viperSet: map[string]interface{}{
					FlagPrometheus:           true,
					FlagPrometheusListenAddr: ":9090",
					FlagMaxOpenConnections:   10,
				},
				validate: func(t *testing.T, nc NodeConfig) {
					assert.NotNil(t, nc.Instrumentation)
					assert.Equal(t, true, nc.Instrumentation.Prometheus)
					assert.Equal(t, ":9090", nc.Instrumentation.PrometheusListenAddr)
					assert.Equal(t, 10, nc.Instrumentation.MaxOpenConnections)
					assert.Equal(t, "rollkit", nc.Instrumentation.Namespace)
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				v := viper.New()
				for key, value := range tc.viperSet {
					v.Set(key, value)
				}

				nc := DefaultNodeConfig
				err := nc.GetViperConfig(v)
				assert.NoError(t, err)

				tc.validate(t, nc)
			})
		}
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

		nc := DefaultNodeConfig
		assert.NoError(t, nc.GetViperConfig(v))

		// Verify that the values have been set correctly
		assert.Equal(t, true, nc.Rollkit.Aggregator)
		assert.Equal(t, "http://da.example.com", nc.Rollkit.DAAddress)
		assert.Equal(t, 5*time.Second, nc.Rollkit.BlockTime)
	})

	// Subtest to test all flags together
	t.Run("AllFlags", func(t *testing.T) {
		cmd := &cobra.Command{}
		AddFlags(cmd)

		v := viper.New()
		assert.NoError(t, v.BindPFlags(cmd.Flags()))

		// Configure all flags
		flagValues := map[string]string{
			FlagAggregator:           "true",
			FlagLazyAggregator:       "true",
			FlagDAAddress:            "http://da.example.com",
			FlagDAAuthToken:          "auth-token",
			FlagBlockTime:            "5s",
			FlagDABlockTime:          "10s",
			FlagDAGasPrice:           "1.5",
			FlagDAGasMultiplier:      "2.0",
			FlagDAStartHeight:        "100",
			FlagDANamespace:          "namespace",
			FlagDASubmitOptions:      "options",
			FlagLight:                "true",
			FlagTrustedHash:          "hash",
			FlagMaxPendingBlocks:     "50",
			FlagDAMempoolTTL:         "20",
			FlagLazyBlockTime:        "30s",
			FlagSequencerAddress:     "localhost:8080",
			FlagSequencerRollupID:    "rollup-id",
			FlagExecutorAddress:      "localhost:9090",
			FlagDBPath:               "custom/db/path",
			FlagPrometheus:           "true",
			FlagPrometheusListenAddr: ":9090",
			FlagMaxOpenConnections:   "10",
		}

		for flag, value := range flagValues {
			assert.NoError(t, cmd.Flags().Set(flag, value))
		}

		// Configure P2P flags directly in Viper
		v.Set("p2p.laddr", "tcp://0.0.0.0:26656")
		v.Set("p2p.seeds", "seed1,seed2")
		v.Set("p2p.blocked_peers", "peer1,peer2")
		v.Set("p2p.allowed_peers", "peer3,peer4")

		nc := DefaultNodeConfig
		assert.NoError(t, nc.GetViperConfig(v))

		// Verify all values
		assert.Equal(t, true, nc.Rollkit.Aggregator)
		assert.Equal(t, true, nc.Rollkit.LazyAggregator)
		assert.Equal(t, "http://da.example.com", nc.Rollkit.DAAddress)
		assert.Equal(t, "auth-token", nc.Rollkit.DAAuthToken)
		assert.Equal(t, 5*time.Second, nc.Rollkit.BlockTime)
		assert.Equal(t, 10*time.Second, nc.Rollkit.DABlockTime)
		assert.Equal(t, 1.5, nc.Rollkit.DAGasPrice)
		assert.Equal(t, 2.0, nc.Rollkit.DAGasMultiplier)
		assert.Equal(t, uint64(100), nc.Rollkit.DAStartHeight)
		assert.Equal(t, "namespace", nc.Rollkit.DANamespace)
		assert.Equal(t, "options", nc.Rollkit.DASubmitOptions)
		assert.Equal(t, true, nc.Rollkit.Light)
		assert.Equal(t, "hash", nc.Rollkit.TrustedHash)
		assert.Equal(t, uint64(50), nc.Rollkit.MaxPendingBlocks)
		assert.Equal(t, uint64(20), nc.Rollkit.DAMempoolTTL)
		assert.Equal(t, 30*time.Second, nc.Rollkit.LazyBlockTime)
		assert.Equal(t, "localhost:8080", nc.Rollkit.SequencerAddress)
		assert.Equal(t, "rollup-id", nc.Rollkit.SequencerRollupID)
		assert.Equal(t, "localhost:9090", nc.Rollkit.ExecutorAddress)
		assert.Equal(t, "custom/db/path", nc.Rollkit.DBPath)

		// P2P config
		assert.Equal(t, "tcp://0.0.0.0:26656", nc.P2P.ListenAddress)
		assert.Equal(t, "seed1,seed2", nc.P2P.Seeds)
		assert.Equal(t, "peer1,peer2", nc.P2P.BlockedPeers)
		assert.Equal(t, "peer3,peer4", nc.P2P.AllowedPeers)

		// Instrumentation config
		assert.NotNil(t, nc.Instrumentation)
		assert.Equal(t, true, nc.Instrumentation.Prometheus)
		assert.Equal(t, ":9090", nc.Instrumentation.PrometheusListenAddr)
		assert.Equal(t, 10, nc.Instrumentation.MaxOpenConnections)
		assert.Equal(t, "rollkit", nc.Instrumentation.Namespace)
	})
}
