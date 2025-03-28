package config

import "errors"

// InstrumentationConfig defines the configuration for metrics reporting.
type InstrumentationConfig struct {
	// When true, Prometheus metrics are served under /metrics on
	// PrometheusListenAddr.
	// Check out the documentation for the list of available metrics.
	Prometheus bool `yaml:"prometheus" comment:"Enable Prometheus metrics"` // When true, Prometheus metrics are served

	// Address to listen for Prometheus collector(s) connections.
	PrometheusListenAddr string `mapstructure:"prometheus_listen_addr" yaml:"prometheus_listen_addr" comment:"Address to listen for Prometheus metrics"`

	// Maximum number of simultaneous connections.
	// If you want to accept a larger number than the default, make sure
	// you increase your OS limits.
	// 0 - unlimited.
	MaxOpenConnections int `mapstructure:"max_open_connections" yaml:"max_open_connections" comment:"Maximum number of simultaneous connections"`

	// Instrumentation namespace.
	Namespace string `yaml:"namespace" comment:"Namespace for metrics"` // Instrumentation namespace

	// When true, pprof endpoints are served under /debug/pprof/ on
	// PprofListenAddr. This enables runtime profiling of the application.
	// Available endpoints include:
	// - /debug/pprof/          - Index page
	// - /debug/pprof/cmdline   - Command line arguments
	// - /debug/pprof/profile   - CPU profile
	// - /debug/pprof/symbol    - Symbol lookup
	// - /debug/pprof/trace     - Execution trace
	// - /debug/pprof/goroutine - Goroutine stack dumps
	// - /debug/pprof/heap      - Heap memory profile
	// - /debug/pprof/mutex     - Mutex contention profile
	// - /debug/pprof/block     - Block profile
	// - /debug/pprof/allocs    - Allocation profile
	Pprof bool `yaml:"pprof" comment:"Enable pprof profiling server"` // When true, pprof endpoints are served

	// Address to listen for pprof connections.
	// Default is ":6060" which is the standard port for pprof.
	PprofListenAddr string `mapstructure:"pprof_listen_addr" yaml:"pprof_listen_addr" comment:"Address to listen for pprof connections"`
}

// DefaultInstrumentationConfig returns a default configuration for metrics
// reporting.
func DefaultInstrumentationConfig() *InstrumentationConfig {
	return &InstrumentationConfig{
		Prometheus:           false,
		PrometheusListenAddr: ":26660",
		MaxOpenConnections:   3,
		Namespace:            "rollkit",
		Pprof:                false,
		PprofListenAddr:      ":6060",
	}
}

// TestInstrumentationConfig returns a default configuration for test environments.
func TestInstrumentationConfig() *InstrumentationConfig {
	return DefaultInstrumentationConfig()
}

// ValidateBasic performs basic validation (checking param bounds, etc.) and
// returns an error if any check fails.
func (cfg *InstrumentationConfig) ValidateBasic() error {
	if cfg.MaxOpenConnections < 0 {
		return errors.New("max_open_connections can't be negative")
	}
	return nil
}

// IsPrometheusEnabled returns true if Prometheus metrics are enabled.
func (cfg *InstrumentationConfig) IsPrometheusEnabled() bool {
	return cfg.Prometheus && cfg.PrometheusListenAddr != ""
}

// IsPprofEnabled returns true if pprof endpoints are enabled.
func (cfg *InstrumentationConfig) IsPprofEnabled() bool {
	return cfg.Pprof
}

// GetPprofListenAddr returns the address to listen for pprof connections.
// If PprofListenAddr is empty, it returns the default pprof port ":6060".
func (cfg *InstrumentationConfig) GetPprofListenAddr() string {
	if cfg.PprofListenAddr == "" {
		return ":6060"
	}
	return cfg.PprofListenAddr
}
