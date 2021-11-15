package conv

import (
	tmcfg "github.com/tendermint/tendermint/config"

	"github.com/celestiaorg/optimint/config"
)

// GetNodeConfig translates Tendermint's configuration into Optimint configuration.
//
// This method only translates configuration, and doesn't verify it. If some option is missing in Tendermint's
// config, it's skipped during translation.
func GetNodeConfig(nodeConf *config.NodeConfig, tmConf *tmcfg.Config) {
	if tmConf != nil {
		nodeConf.RootDir = tmConf.RootDir
		nodeConf.DBPath = tmConf.DBPath
		if tmConf.P2P != nil {
			nodeConf.P2P.ListenAddress = tmConf.P2P.ListenAddress
			nodeConf.P2P.Seeds = tmConf.P2P.Seeds
		}
		if tmConf.RPC != nil {
			nodeConf.RPC.ListenAddress = tmConf.RPC.ListenAddress
			nodeConf.RPC.CORSAllowedOrigins = tmConf.RPC.CORSAllowedOrigins
			nodeConf.RPC.CORSAllowedMethods = tmConf.RPC.CORSAllowedMethods
			nodeConf.RPC.CORSAllowedHeaders = tmConf.RPC.CORSAllowedHeaders
			nodeConf.RPC.MaxOpenConnections = tmConf.RPC.MaxOpenConnections
			nodeConf.RPC.TLSCertFile = tmConf.RPC.TLSCertFile
			nodeConf.RPC.TLSKeyFile = tmConf.RPC.TLSKeyFile
		}
	}
}
