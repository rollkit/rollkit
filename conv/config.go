package conv

import (
	tmcfg "github.com/tendermint/tendermint/config"

	"github.com/celestiaorg/optimint/config"
)

// GetNodeConfig translates Tendermint's configuration into Optimint configuration.
//
// This method only translates configuration, and doesn't verify it. If some option is missing in Tendermint's
// config, it's skipped during translation.
func GetNodeConfig(cfg *tmcfg.Config) config.NodeConfig {
	nodeConf := config.NodeConfig{}

	if cfg != nil {
		nodeConf.RootDir = cfg.RootDir
		nodeConf.DBPath = cfg.DBPath
		if cfg.P2P != nil {
			nodeConf.P2P.ListenAddress = cfg.P2P.ListenAddress
			nodeConf.P2P.Seeds = cfg.P2P.Seeds
		}
	}

	return nodeConf
}
