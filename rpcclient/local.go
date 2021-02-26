package rpcclient

import (
	"context"
	"fmt"
	tmbytes "github.com/lazyledger/lazyledger-core/libs/bytes"
	tmpubsub "github.com/lazyledger/lazyledger-core/libs/pubsub"
	tmquery "github.com/lazyledger/lazyledger-core/libs/pubsub/query"
	rpcclient "github.com/lazyledger/lazyledger-core/rpc/client"
	"github.com/lazyledger/lazyledger-core/rpc/core"
	ctypes "github.com/lazyledger/lazyledger-core/rpc/core/types"
	rpctypes "github.com/lazyledger/lazyledger-core/rpc/jsonrpc/types"
	"github.com/lazyledger/lazyledger-core/types"
	"github.com/lazyledger/optimint/node"
	"time"
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
	q, err := tmquery.New(query)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query: %w", err)
	}

	outCap := 1
	if len(outCapacity) > 0 {
		outCap = outCapacity[0]
	}

	var sub types.Subscription
	if outCap > 0 {
		sub, err = l.EventBus.Subscribe(ctx, subscriber, q, outCap)
	} else {
		sub, err = l.EventBus.SubscribeUnbuffered(ctx, subscriber, q)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe: %w", err)
	}

	outc := make(chan ctypes.ResultEvent, outCap)
	go l.eventsRoutine(sub, subscriber, q, outc)

	return outc, nil
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

func (l *Local) eventsRoutine(sub types.Subscription, subscriber string, q tmpubsub.Query, outc chan<- ctypes.ResultEvent) {
	for {
		select {
		case msg := <-sub.Out():
			result := ctypes.ResultEvent{Query: q.String(), Data: msg.Data(), Events: msg.Events()}
			if cap(outc) == 0 {
				outc <- result
			} else {
				select {
				case outc <- result:
				default:
					l.Logger.Error("wanted to publish ResultEvent, but out channel is full", "result", result, "query", result.Query)
				}
			}
		case <-sub.Cancelled():
			if sub.Err() == tmpubsub.ErrUnsubscribed {
				return
			}

			l.Logger.Error("subscription was cancelled, resubscribing...", "err", sub.Err(), "query", q.String())
			sub = l.resubscribe(subscriber, q)
			if sub == nil { // client was stopped
				return
			}
		case <-l.Quit():
			return
		}
	}
}

// Try to resubscribe with exponential backoff.
func (l *Local) resubscribe(subscriber string, q tmpubsub.Query) types.Subscription {
	attempts := 0
	for {
		if !l.IsRunning() {
			return nil
		}

		sub, err := l.EventBus.Subscribe(context.Background(), subscriber, q)
		if err == nil {
			return sub
		}

		attempts++
		time.Sleep((10 << uint(attempts)) * time.Millisecond) // 10ms -> 20ms -> 40ms
	}
}
