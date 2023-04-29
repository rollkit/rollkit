package rpc

import (
	"cosmossdk.io/store/iavl"
	//storetypes "cosmossdk.io/store/types"
	tmdb "github.com/tendermint/db"
	"github.com/tendermint/libs/log"
	"github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
)

var _ types.Application = (*Application)(nil)

type Application struct {
	types.BaseApplication
	store iavl.Store
}

func NewMerkleApp() Application {
	db := tmdb.NewMemDB()
	log := log.NewNopLogger()
	store := iavl.LoadStore()
	return Application{}
}
