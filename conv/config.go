package conv

import (
	cmcfg "github.com/cometbft/cometbft/config"

	"github.com/rollkit/rollkit/config"
)

// GetNodeConfig translates Tendermint's configuration into Rollkit configuration.
//
// This method only translates configuration, and doesn't verify it. If some option is missing in Tendermint's
// config, it's skipped during translation.
func GetNodeConfig(nodeConf *config.NodeConfig, cmConf *cmcfg.Config) {
	if cmConf != nil {
		nodeConf.RootDir = cmConf.RootDir
		nodeConf.DBPath = cmConf.DBPath
		if cmConf.P2P != nil {
			nodeConf.P2P.ListenAddress = cmConf.P2P.ListenAddress
			nodeConf.P2P.Seeds = cmConf.P2P.Seeds
		}
		if cmConf.RPC != nil {
			nodeConf.RPC.ListenAddress = cmConf.RPC.ListenAddress
			nodeConf.RPC.CORSAllowedOrigins = cmConf.RPC.CORSAllowedOrigins
			nodeConf.RPC.CORSAllowedMethods = cmConf.RPC.CORSAllowedMethods
			nodeConf.RPC.CORSAllowedHeaders = cmConf.RPC.CORSAllowedHeaders
			nodeConf.RPC.MaxOpenConnections = cmConf.RPC.MaxOpenConnections
			nodeConf.RPC.TLSCertFile = cmConf.RPC.TLSCertFile
			nodeConf.RPC.TLSKeyFile = cmConf.RPC.TLSKeyFile
		}
	}
}
