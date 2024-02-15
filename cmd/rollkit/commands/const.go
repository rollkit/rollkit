package commands

import (
	"os"

	cmCfg "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/libs/log"
)

var (
	// GitSHA is set at build time
	GitSHA string

	// Version is set at build time
	Version string

	// defaultConfig is the default configuration for the Rollkit pulled
	// from CometBFT
	defaultConfig = cmCfg.DefaultConfig()

	// defaultLogger is the default logger for the Rollkit pulled from CometBFT
	defaultLogger = log.NewTMLogger(log.NewSyncWriter(os.Stdout))
)
