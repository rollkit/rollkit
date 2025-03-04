package config

import "time"

// ConsensusConfig stores configuration related to consensus.
type ConsensusConfig struct {
	// CreateEmptyBlocks determines if empty blocks should be created
	CreateEmptyBlocks bool `mapstructure:"create_empty_blocks"`
	// CreateEmptyBlocksInterval sets the interval between empty blocks
	CreateEmptyBlocksInterval time.Duration `mapstructure:"create_empty_blocks_interval"`
	// DoubleSignCheckHeight sets the height until which double sign check should be performed
	DoubleSignCheckHeight int64 `mapstructure:"double_sign_check_height"`
}
