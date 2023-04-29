package rpc

import (
	"bytes"
	"fmt"

	dbc "github.com/cosmos/cosmos-db"

	ics23 "github.com/confio/ics23/go"
	"github.com/cosmos/iavl"
	"github.com/tendermint/tendermint/abci/example/code"
	"github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/version"
)

type State struct {
	db      dbc.DB
	tree    iavl.Tree
	Height  int64  `json:"height"`
	AppHash []byte `json:"app_hash"`
}

func initState(db dbc.DB) State {
	var state State
	var err error
	state.tree, err = iavl.NewMutableTree(db, 10, false)
	if err != nil {
		panic(err)
	}
	return state
}

//---------------------------------------------------

var _ types.Application = (*Application)(nil)

type Application struct {
	types.BaseApplication

	state        State
	RetainBlocks int64 // blocks to retain after commit (via ResponseCommit.RetainHeight)
}

func NewKVApplication() *Application {
	state := initState(dbc.NewMemDB())
	return &Application{state: state}
}

func (app *Application) Info(req types.RequestInfo) (resInfo types.ResponseInfo) {
	return types.ResponseInfo{
		Data:             fmt.Sprintf("{\"apphash\":%v}", app.state.AppHash),
		Version:          version.ABCIVersion,
		AppVersion:       1,
		LastBlockHeight:  app.state.Height,
		LastBlockAppHash: app.state.AppHash,
	}
}

// tx is either "key=value" or just arbitrary bytes
func (app *Application) DeliverTx(req types.RequestDeliverTx) types.ResponseDeliverTx {
	var key, value []byte
	parts := bytes.Split(req.Tx, []byte("="))
	if len(parts) == 2 {
		key, value = parts[0], parts[1]
	} else {
		key, value = req.Tx, req.Tx
	}

	_, err := app.state.tree.Set(key, value)
	if err != nil {
		panic(err)
	}

	events := []types.Event{
		{
			Type: "app",
			Attributes: []types.EventAttribute{
				{Key: []byte("creator"), Value: []byte("Cosmoshi Netowoko"), Index: true},
				{Key: []byte("key"), Value: key, Index: true},
				{Key: []byte("index_key"), Value: []byte("index is working"), Index: true},
				{Key: []byte("noindex_key"), Value: []byte("index is working"), Index: false},
			},
		},
	}

	return types.ResponseDeliverTx{Code: code.CodeTypeOK, Events: events}
}

func (app *Application) CheckTx(req types.RequestCheckTx) types.ResponseCheckTx {
	return types.ResponseCheckTx{Code: code.CodeTypeOK, GasWanted: 1}
}

func (app *Application) Commit() types.ResponseCommit {
	appHash, err := app.state.tree.Hash()
	if err != nil {
		panic("couldn't get tree hash")
	}
	app.state.AppHash = appHash
	app.state.Height += 1
	resp := types.ResponseCommit{Data: app.state.AppHash}
	return resp
}

// Returns an associated value or nil if missing.
func (app *Application) Query(reqQuery types.RequestQuery) (resQuery types.ResponseQuery) {

	resQuery.Key = reqQuery.Data
	value, err := app.state.db.Get(reqQuery.Data)
	if err != nil {
		panic(err)
	}
	if value == nil {
		resQuery.Log = "does not exist"
	} else {
		resQuery.Log = "exists"
	}
	resQuery.Value = value
	resQuery.Height = app.state.Height

	if reqQuery.Prove {
		value, err := app.state.db.Get(reqQuery.Data)
		if err != nil {
			panic(err)
		}
		var proof ics23.CommitmentProof
		if value == nil {
			resQuery.Log = "does not exist"
			proof, err = app.state.tree.GetNonMembershipProof(reqQuery.Data)
			if err != nil {
				panic("Error getting non membership proof")
			}
		} else {
			resQuery.Log = "exists"
			proof, err = app.state.tree.GetMembershipProof(reqQuery.Data)
			if err != nil {
				panic("Error getting membership proof")
			}
		}
		resQuery.Index = -1 // TODO make Proof return index
		resQuery.Key = reqQuery.Data
		resQuery.Value = value
		resQuery.Height = app.state.Height

		return
	}

	return resQuery
}
