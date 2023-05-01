package rpc

import (
	"testing"

	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/crypto/merkle"
)

func TestKvApp(t *testing.T) {
	require := require.New(t)
	app := NewMerkleApp()
	app.DeliverTx(types.RequestDeliverTx{
		Tx: []byte("cnode=cool"),
	})
	app.Commit()
	response := app.Query(types.RequestQuery{
		Path:   "/key",
		Data:   []byte("cnode"),
		Height: 1,
		Prove:  true,
	})
	decoder := merkle.NewProofRuntime()
	decoder.RegisterOpDecoder("ics23:iavl", storetypes.CommitmentOpDecoder)
	err := decoder.VerifyValue(response.ProofOps, app.appHash, "/cnode", []byte("cool"))
	require.NoError(err)
	err = decoder.VerifyValue(response.ProofOps, app.appHash, "/cnode2", []byte("cool"))
	require.Error(err)
	err = decoder.VerifyValue(response.ProofOps, app.appHash, "/cnode", []byte("cringe"))
	require.Error(err)
	app.DeliverTx(types.RequestDeliverTx{
		Tx: []byte("cnode=based"),
	})
	app.Commit()
	response = app.Query(types.RequestQuery{
		Path:   "/key",
		Data:   []byte("cnode"),
		Prove:  true,
		Height: 2,
	})
	err = decoder.VerifyValue(response.ProofOps, app.appHash, "/cnode", []byte("based"))
	require.NoError(err)
	err = decoder.VerifyValue(response.ProofOps, app.appHash, "/cnode", []byte("cringee"))
	require.Error(err)
}
