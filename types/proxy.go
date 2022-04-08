package types

import (
	"context"
	abciclient "github.com/tendermint/tendermint/abci/client"
	"github.com/tendermint/tendermint/abci/types"
)

type AppConnMempool interface {
	SetResponseCallback(abciclient.Callback)
	Error() error

	CheckTxAsync(context.Context, types.RequestCheckTx) (*abciclient.ReqRes, error)
	CheckTxSync(context.Context, types.RequestCheckTx) (*types.ResponseCheckTx, error)

	FlushAsync(context.Context) (*abciclient.ReqRes, error)
	FlushSync(context.Context) error
}
