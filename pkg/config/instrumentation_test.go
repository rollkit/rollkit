package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultInstrumentationConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultInstrumentationConfig()

	assert.False(t, cfg.Prometheus)
	assert.Equal(t, ":26660", cfg.PrometheusListenAddr)
	assert.Equal(t, 3, cfg.MaxOpenConnections)
	assert.Equal(t, "rollkit", cfg.Namespace)
	assert.False(t, cfg.Pprof)
	assert.Equal(t, ":6060", cfg.PprofListenAddr)
}

func TestInstrumentationConfigValidateBasic(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		cfg       *InstrumentationConfig
		expectErr bool
	}{
		{
			"valid config",
			&InstrumentationConfig{
				MaxOpenConnections: 3,
			},
			false,
		},
		{
			"negative max open connections",
			&InstrumentationConfig{
				MaxOpenConnections: -1,
			},
			true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.ValidateBasic()
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsPrometheusEnabled(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		cfg      *InstrumentationConfig
		expected bool
	}{
		{
			"prometheus enabled with address",
			&InstrumentationConfig{
				Prometheus:           true,
				PrometheusListenAddr: ":26660",
			},
			true,
		},
		{
			"prometheus enabled without address",
			&InstrumentationConfig{
				Prometheus:           true,
				PrometheusListenAddr: "",
			},
			false,
		},
		{
			"prometheus disabled with address",
			&InstrumentationConfig{
				Prometheus:           false,
				PrometheusListenAddr: ":26660",
			},
			false,
		},
		{
			"prometheus disabled without address",
			&InstrumentationConfig{
				Prometheus:           false,
				PrometheusListenAddr: "",
			},
			false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.cfg.IsPrometheusEnabled())
		})
	}
}

func TestIsPprofEnabled(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		cfg      *InstrumentationConfig
		expected bool
	}{
		{
			"pprof enabled",
			&InstrumentationConfig{
				Pprof: true,
			},
			true,
		},
		{
			"pprof disabled",
			&InstrumentationConfig{
				Pprof: false,
			},
			false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.cfg.IsPprofEnabled())
		})
	}
}

func TestGetPprofListenAddr(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		cfg      *InstrumentationConfig
		expected string
	}{
		{
			"pprof address set",
			&InstrumentationConfig{
				PprofListenAddr:      ":8080",
				PrometheusListenAddr: ":26660",
			},
			":8080",
		},
		{
			"pprof address not set",
			&InstrumentationConfig{
				PprofListenAddr:      "",
				PrometheusListenAddr: ":26660",
			},
			":6060",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.cfg.GetPprofListenAddr())
		})
	}
}
