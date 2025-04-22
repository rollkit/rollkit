package config

const (
	// Pruning configuration flags

	// FlagPruningStrategy is a flag for specifying strategy for pruning block store
	FlagPruningStrategy = "rollkit.node.pruning.strategy"
	// FlagPruningKeepRecent is a flag for specifying how many blocks need to keep in store
	FlagPruningKeepRecent = "rollkit.node.pruning.keep_recent"
	// FlagPruningInterval is a flag for specifying how offen prune blocks store
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
	Strategy   string `mapstructure:"strategy" yaml:"strategy" comment:"Strategy determines the pruning approach (none, default, custom)"`
	KeepRecent uint64 `mapstructure:"keep_recent" yaml:"keep_recent" comment:"Number of recent blocks to keep, used in \"default\" and \"custom\" strategies"`
	Interval   uint64 `mapstructure:"interval" yaml:"interval" comment:"how offen the pruning process should run"`

	// todo: support volume-based strategy
	// VolumeConfig specifies configuration for volume-based storage
	// VolumeConfig *VolumeStorageConfig `mapstructure:"volume_config" yaml:"volume_config"`
}
