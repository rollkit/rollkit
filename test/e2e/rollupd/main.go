package main

import (
	"os"

	svrcmd "github.com/cosmos/cosmos-sdk/server/cmd"
	"github.com/evan-forbes/rollup/app"
	"github.com/tendermint/spm/cosmoscmd"
)

func main() {
	rootCmd, _ := cosmoscmd.NewRootCmd(
		app.Name,
		app.AccountAddressPrefix,
		app.DefaultNodeHome,
		app.Name,
		app.ModuleBasics,
		app.New,
	)

	encCfg := cosmoscmd.MakeEncodingConfig(app.ModuleBasics)

	rollupCreator := appCreator{
		encodingConfig: encCfg,
		buildApp:       app.New,
	}

	rootCmd.AddCommand(StartCmd(rollupCreator.newApp, app.DefaultNodeHome))

	if err := svrcmd.Execute(rootCmd, app.DefaultNodeHome); err != nil {
		os.Exit(1)
	}
}
