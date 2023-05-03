package rpc

import (
	"bytes"
	"fmt"

	"github.com/cosmos/cosmos-sdk/store/iavl"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	"github.com/tendermint/tendermint/abci/example/code"
	"github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	tmdb "github.com/tendermint/tm-db"
)

var _ types.Application = (*Application)(nil)

type Application struct {
	types.BaseApplication
	store   storetypes.CommitKVStore
	appHash []byte
}

func NewMerkleApp() *Application {
	db := tmdb.NewMemDB()
	key := storetypes.NewKVStoreKey("data")
	commitID := storetypes.CommitID{
		Version: 0,
		Hash:    []byte(""),
	}
	log := log.NewNopLogger()
	store, err := iavl.LoadStore(db, log, key, commitID, false, 10, true)
	if err != nil {
		panic("Unable to create IAVL store")
	}
	appHash := make([]byte, 8)
	return &Application{
		store:   store,
		appHash: appHash,
	}
}

func (a *Application) DeliverTx(req types.RequestDeliverTx) types.ResponseDeliverTx {

	var key, value string
	parts := bytes.Split(req.Tx, []byte("="))
	if len(parts) == 2 {
		key, value = string(parts[0]), string(parts[1])
	} else {
		key, value = string(req.Tx), string(req.Tx)
	}
	a.store.Set([]byte(key), []byte(value))

	return types.ResponseDeliverTx{
		Code: code.CodeTypeOK,
	}
}

func (a *Application) Commit() types.ResponseCommit {
	resp := a.store.Commit()
	a.appHash = resp.Hash
	return types.ResponseCommit{
		Data: resp.Hash,
	}
}

func (a Application) Query(req types.RequestQuery) types.ResponseQuery {
	fmt.Println("Received query")
	fmt.Println(req.Path)
	return a.store.(*iavl.Store).Query(req)
}
