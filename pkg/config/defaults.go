package config

import (
	"os"
	"path/filepath"
	"time"
)

const (
	// ConfigFileName is the base name of the rollkit configuration file without extension.
	ConfigFileName = "rollkit"
	// ConfigExtension is the file extension for the configuration file without the leading dot.
	ConfigExtension = "yaml"
	// ConfigPath is the filename for the rollkit configuration file.
	ConfigName = ConfigFileName + "." + ConfigExtension
	// AppConfigDir is the directory name for the app configuration.
	AppConfigDir = "config"
)

// DefaultRootDir returns the default root directory for rollkit
var DefaultRootDir = DefaultRootDirWithName(ConfigFileName)

// DefaultRootDirWithName returns the default root directory for an application,
// based on the app name and the user's home directory
func DefaultRootDirWithName(appName string) string {
	if appName == "" {
		appName = ConfigFileName
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, "."+appName)
}

// DefaultConfig keeps default values of NodeConfig
var DefaultConfig = Config{
	RootDir: DefaultRootDir,
	DBPath:  "data",
	ChainID: "rollkit-test",
	P2P: P2PConfig{
		ListenAddress: "/ip4/0.0.0.0/tcp/7676",
		Peers:         "",
	},
	Node: NodeConfig{
		Aggregator:        false,
		BlockTime:         DurationWrapper{1 * time.Second},
		LazyMode:          false,
		LazyBlockInterval: DurationWrapper{60 * time.Second},
		Light:             false,
		TrustedHash:       "",
	},
	DA: DAConfig{
		Address:       "http://localhost:7980",
		BlockTime:     DurationWrapper{15 * time.Second},
		GasPrice:      -1,
		GasMultiplier: 0,
	},
	Instrumentation: DefaultInstrumentationConfig(),
	Log: LogConfig{
		Level:  "info",
		Format: "text",
		Trace:  false,
	},
	Signer: SignerConfig{
		SignerType: "file",
		SignerPath: "config",
	},
	RPC: RPCConfig{
		Address: "127.0.0.1:7331",
	},
}
