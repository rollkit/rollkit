package config

import (
	"errors"
)

const (
	// Pruning configuration flags

	// FlagPruningStrategy is a flag for specifying strategy for pruning block store
	FlagPruningStrategy = "rollkit.node.pruning.strategy"
	// FlagPruningKeepRecent is a flag for specifying how many blocks need to keep in store
	FlagPruningKeepRecent = "rollkit.node.pruning.keep_recent"
	// FlagPruningInterval is a flag for specifying how often prune blocks store
	FlagPruningInterval = "rollkit.node.pruning.interval"
)

const (
	PruningConfigStrategyNone       = "none"
	PruningConfigStrategyDefault    = "default"
	PruningConfigStrategyEverything = "everything"
	PruningConfigStrategyCustom     = "custom"
)

var (
	PruningConfigNone = PruningConfig{
		Strategy:   PruningConfigStrategyNone,
		KeepRecent: 0,
		Interval:   0,
	}
	PruningConfigDefault = PruningConfig{
		Strategy:   PruningConfigStrategyDefault,
		KeepRecent: 362880,
		Interval:   10,
	}
	PruningConfigEverything = PruningConfig{
		Strategy:   PruningConfigStrategyEverything,
		KeepRecent: 2,
		Interval:   10,
	}
	PruningConfigCustom = PruningConfig{
		Strategy:   PruningConfigStrategyCustom,
		KeepRecent: 100,
		Interval:   100,
	}
)

// PruningConfig allows node operators to manage storage
type PruningConfig struct {
	// todo: support volume-based strategy
	Strategy   string `mapstructure:"strategy" yaml:"strategy" comment:"Strategy determines the pruning approach (none, default, everything, custom)"`
	KeepRecent uint64 `mapstructure:"keep_recent" yaml:"keep_recent" comment:"Number of recent blocks to keep, used in \"custom\" strategy, must be greater or equal than 2"`
	Interval   uint64 `mapstructure:"interval" yaml:"interval" comment:"How often the pruning process should run, used in \"custom\" strategy"`

	// todo: support volume-based strategy
	// VolumeConfig specifies configuration for volume-based storage
	// VolumeConfig *VolumeStorageConfig `mapstructure:"volume_config" yaml:"volume_config"`
}

func (p PruningConfig) Validate() error {
	// Only Custom strategy requires validation.
	if p.Strategy != PruningConfigStrategyCustom {
		return nil
	}

	if p.KeepRecent < 2 {
		return errors.New("keep_recent must be greater or equal than 2 for custom pruning strategy")
	}

	return nil
}

func GetPruningConfigFromStrategy(strategy string) PruningConfig {
	switch strategy {
	case PruningConfigStrategyDefault:
		return PruningConfigDefault
	case PruningConfigStrategyEverything:
		return PruningConfigEverything
	default:
		return PruningConfigNone
	}
}
