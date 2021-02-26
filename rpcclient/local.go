package rpcclient

import (
	"context"
	"fmt"
	tmbytes "github.com/lazyledger/lazyledger-core/libs/bytes"
	tmquery "github.com/lazyledger/lazyledger-core/libs/pubsub/query"
	rpcclient "github.com/lazyledger/lazyledger-core/rpc/client"
	"github.com/lazyledger/lazyledger-core/rpc/core"
	ctypes "github.com/lazyledger/lazyledger-core/rpc/core/types"
	rpctypes "github.com/lazyledger/lazyledger-core/rpc/jsonrpc/types"
	"github.com/lazyledger/lazyledger-core/types"
	"github.com/lazyledger/optimint/node"
)

var _ rpcclient.Client = &Local{}

type Local struct {
	*types.EventBus
	ctx      *rpctypes.Context
}

func NewLocal(node *node.Node) *Local {
	if err := node.ConfigureRPC(); err != nil {
		node.Logger.Error("Error configuring RPC", "err", err)
	}
	return &Local{
		EventBus: node.EventBus(),
		ctx:      &rpctypes.Context{},
	}
}

func (l Local) ABCIInfo(ctx context.Context) (*ctypes.ResultABCIInfo, error) {
	return core.ABCIInfo(l.ctx)
}

func (l Local) ABCIQuery(ctx context.Context, path string, data tmbytes.HexBytes) (*ctypes.ResultABCIQuery, error) {
	return l.ABCIQueryWithOptions(ctx, path, data, rpcclient.DefaultABCIQueryOptions)
}

func (l Local) ABCIQueryWithOptions(ctx context.Context, path string, data tmbytes.HexBytes, opts rpcclient.ABCIQueryOptions) (*ctypes.ResultABCIQuery, error) {
	return core.ABCIQuery(l.ctx, path, data, opts.Height, opts.Prove)
}

func (l Local) BroadcastTxCommit(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTxCommit, error) {
	return core.BroadcastTxCommit(l.ctx, tx)
}

func (l Local) BroadcastTxAsync(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTx, error) {
	return core.BroadcastTxAsync(l.ctx, tx)
}

func (l Local) BroadcastTxSync(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTx, error) {
	return core.BroadcastTxSync(l.ctx, tx)
}

func (l Local) Subscribe(ctx context.Context, subscriber, query string, outCapacity ...int) (out <-chan ctypes.ResultEvent, err error) {
	panic("implement me")
}

func (l Local) Unsubscribe(ctx context.Context, subscriber, query string) error {
	q, err := tmquery.New(query)
	if err != nil {
		return fmt.Errorf("failed to parse query: %w", err)
	}
	return l.EventBus.Unsubscribe(ctx, subscriber, q)
}

func (l Local) Genesis(ctx context.Context) (*ctypes.ResultGenesis, error) {
	return core.Genesis(l.ctx)
}

func (l Local) BlockchainInfo(ctx context.Context, minHeight, maxHeight int64) (*ctypes.ResultBlockchainInfo, error) {
	return core.BlockchainInfo(l.ctx, minHeight, maxHeight)
}

func (l Local) NetInfo(ctx context.Context) (*ctypes.ResultNetInfo, error) {
	return core.NetInfo(l.ctx)
}

func (l Local) DumpConsensusState(ctx context.Context) (*ctypes.ResultDumpConsensusState, error) {
	return core.DumpConsensusState(l.ctx)
}

func (l Local) ConsensusState(ctx context.Context) (*ctypes.ResultConsensusState, error) {
	return core.ConsensusState(l.ctx)
}

func (l Local) ConsensusParams(ctx context.Context, height *int64) (*ctypes.ResultConsensusParams, error) {
	return core.ConsensusParams(l.ctx, height)
}

func (l Local) Health(ctx context.Context) (*ctypes.ResultHealth, error) {
	return core.Health(l.ctx)
}

func (l Local) Block(ctx context.Context, height *int64) (*ctypes.ResultBlock, error) {
	return core.Block(l.ctx, height)
}

func (l Local) BlockByHash(ctx context.Context, hash []byte) (*ctypes.ResultBlock, error) {
	return core.BlockByHash(l.ctx, hash)
}

func (l Local) BlockResults(ctx context.Context, height *int64) (*ctypes.ResultBlockResults, error) {
	return core.BlockResults(l.ctx, height)
}

func (l Local) Commit(ctx context.Context, height *int64) (*ctypes.ResultCommit, error) {
	return core.Commit(l.ctx, height)
}

func (l Local) Validators(ctx context.Context, height *int64, page, perPage *int) (*ctypes.ResultValidators, error) {
	return core.Validators(l.ctx, height, page, perPage)
}

func (l Local) Tx(ctx context.Context, hash []byte, prove bool) (*ctypes.ResultTx, error) {
	return core.Tx(l.ctx, hash, prove)
}

func (l Local) TxSearch(ctx context.Context, query string, prove bool, page, perPage *int, orderBy string) (*ctypes.ResultTxSearch, error) {
	return core.TxSearch(l.ctx, query, prove, page, perPage, orderBy)
}

func (l Local) Status(ctx context.Context) (*ctypes.ResultStatus, error) {
	return core.Status(l.ctx)
}

func (l Local) BroadcastEvidence(ctx context.Context, evidence types.Evidence) (*ctypes.ResultBroadcastEvidence, error) {
	return core.BroadcastEvidence(l.ctx, evidence)
}

func (l Local) UnconfirmedTxs(ctx context.Context, limit *int) (*ctypes.ResultUnconfirmedTxs, error) {
	return core.UnconfirmedTxs(l.ctx, limit)
}

func (l Local) NumUnconfirmedTxs(ctx context.Context) (*ctypes.ResultUnconfirmedTxs, error) {
	return core.NumUnconfirmedTxs(l.ctx)
}

func (l Local) CheckTx(ctx context.Context, tx types.Tx) (*ctypes.ResultCheckTx, error) {
	return core.CheckTx(l.ctx, tx)
}


