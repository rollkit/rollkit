package rpc

import (
	"fmt"
	"os"
	"testing"

	"cosmossdk.io/log"
	"github.com/tendermint/tendermint/abci/types"
)

func TestKvApp(t *testing.T) {
	testLogger := log.NewLogger(os.Stdout)
	app := NewMerkleApp(testLogger)
	app.DeliverTx(types.RequestDeliverTx{
		Tx: []byte("cnode=cool"),
	})
	fmt.Println("apphash")
	fmt.Println(app.appHash)
	app.Commit()
	fmt.Println("apphash")
	fmt.Println(app.appHash)
}
