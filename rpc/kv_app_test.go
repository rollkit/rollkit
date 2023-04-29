package rpc

import (
	"testing"

	"github.com/tendermint/tendermint/abci/types"
	//"github.com/tendermint/tendermint/abci/types"
)

func TestKvApp(t *testing.T) {
	app := NewKVApplication()
	println(app.state.AppHash)
	app.DeliverTx(types.RequestDeliverTx{
		Tx: []byte("connor=cool"),
	})
	app.Commit()
	println(app.state.AppHash)
	app.DeliverTx(types.RequestDeliverTx{
		Tx: []byte("connor=cool"),
	})
	app.Commit()
	println(app.state.AppHash)
	app.DeliverTx(types.RequestDeliverTx{
		Tx: []byte("cnode=cool"),
	})
	app.Commit()
	println(app.state.AppHash)
}
