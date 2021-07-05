package rpcclient

import (
	"context"
	"errors"
	"fmt"
	"time"

	abci "github.com/lazyledger/lazyledger-core/abci/types"
	"github.com/lazyledger/lazyledger-core/config"
	tmbytes "github.com/lazyledger/lazyledger-core/libs/bytes"
	tmpubsub "github.com/lazyledger/lazyledger-core/libs/pubsub"
	tmquery "github.com/lazyledger/lazyledger-core/libs/pubsub/query"
	"github.com/lazyledger/lazyledger-core/proxy"
	rpcclient "github.com/lazyledger/lazyledger-core/rpc/client"
	ctypes "github.com/lazyledger/lazyledger-core/rpc/core/types"
	rpctypes "github.com/lazyledger/lazyledger-core/rpc/jsonrpc/types"
	"github.com/lazyledger/lazyledger-core/types"

	"github.com/lazyledger/optimint/mempool"
	"github.com/lazyledger/optimint/node"
)

const (
	// TODO(tzdybal): make this configurable
	subscribeTimeout = 5 * time.Second
)

var _ rpcclient.Client = &Local{}

type Local struct {
	*types.EventBus
	ctx    *rpctypes.Context
	config *config.RPCConfig

	node *node.Node
}

func NewLocal(node *node.Node) *Local {
	return &Local{
		EventBus: node.EventBus(),
		ctx:      &rpctypes.Context{},
		config:   config.DefaultRPCConfig(),
		node:     node,
	}
}

func (l *Local) ABCIInfo(ctx context.Context) (*ctypes.ResultABCIInfo, error) {
	resInfo, err := l.query().InfoSync(ctx, proxy.RequestInfo)
	if err != nil {
		return nil, err
	}
	return &ctypes.ResultABCIInfo{Response: *resInfo}, nil
}

func (l *Local) ABCIQuery(ctx context.Context, path string, data tmbytes.HexBytes) (*ctypes.ResultABCIQuery, error) {
	return l.ABCIQueryWithOptions(ctx, path, data, rpcclient.DefaultABCIQueryOptions)
}

func (l *Local) ABCIQueryWithOptions(ctx context.Context, path string, data tmbytes.HexBytes, opts rpcclient.ABCIQueryOptions) (*ctypes.ResultABCIQuery, error) {
	resQuery, err := l.query().QuerySync(ctx, abci.RequestQuery{
		Path:   path,
		Data:   data,
		Height: opts.Height,
		Prove:  opts.Prove,
	})
	if err != nil {
		return nil, err
	}
	l.Logger.Info("ABCIQuery", "path", path, "data", data, "result", resQuery)
	return &ctypes.ResultABCIQuery{Response: *resQuery}, nil
}

// BroadcastTxCommit returns with the responses from CheckTx and DeliverTx.
// More: https://docs.tendermint.com/master/rpc/#/Tx/broadcast_tx_commit
func (l *Local) BroadcastTxCommit(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTxCommit, error) {
	// This implementation corresponds to Tendermints implementation from rpc/core/mempool.go.
	// ctx.RemoteAddr godoc: If neither HTTPReq nor WSConn is set, an empty string is returned.
	// This code is a local client, so we can assume that subscriber is ""
	subscriber := "" //ctx.RemoteAddr()

	if l.EventBus.NumClients() >= l.config.MaxSubscriptionClients {
		return nil, fmt.Errorf("max_subscription_clients %d reached", l.config.MaxSubscriptionClients)
	} else if l.EventBus.NumClientSubscriptions(subscriber) >= l.config.MaxSubscriptionsPerClient {
		return nil, fmt.Errorf("max_subscriptions_per_client %d reached", l.config.MaxSubscriptionsPerClient)
	}

	// Subscribe to tx being committed in block.
	subCtx, cancel := context.WithTimeout(ctx, subscribeTimeout)
	defer cancel()
	q := types.EventQueryTxFor(tx)
	deliverTxSub, err := l.EventBus.Subscribe(subCtx, subscriber, q)
	if err != nil {
		err = fmt.Errorf("failed to subscribe to tx: %w", err)
		l.Logger.Error("Error on broadcast_tx_commit", "err", err)
		return nil, err
	}
	defer func() {
		if err := l.EventBus.Unsubscribe(context.Background(), subscriber, q); err != nil {
			l.Logger.Error("Error unsubscribing from eventBus", "err", err)
		}
	}()

	// Broadcast tx and wait for CheckTx result
	checkTxResCh := make(chan *abci.Response, 1)
	err = l.node.Mempool.CheckTx(tx, func(res *abci.Response) {
		checkTxResCh <- res
	}, mempool.TxInfo{Context: ctx})
	if err != nil {
		l.Logger.Error("Error on broadcastTxCommit", "err", err)
		return nil, fmt.Errorf("error on broadcastTxCommit: %v", err)
	}
	checkTxResMsg := <-checkTxResCh
	checkTxRes := checkTxResMsg.GetCheckTx()
	if checkTxRes.Code != abci.CodeTypeOK {
		return &ctypes.ResultBroadcastTxCommit{
			CheckTx:   *checkTxRes,
			DeliverTx: abci.ResponseDeliverTx{},
			Hash:      tx.Hash(),
		}, nil
	}

	// Wait for the tx to be included in a block or timeout.
	select {
	case msg := <-deliverTxSub.Out(): // The tx was included in a block.
		deliverTxRes := msg.Data().(types.EventDataTx)
		return &ctypes.ResultBroadcastTxCommit{
			CheckTx:   *checkTxRes,
			DeliverTx: deliverTxRes.Result,
			Hash:      tx.Hash(),
			Height:    deliverTxRes.Height,
		}, nil
	case <-deliverTxSub.Cancelled():
		var reason string
		if deliverTxSub.Err() == nil {
			reason = "Tendermint exited"
		} else {
			reason = deliverTxSub.Err().Error()
		}
		err = fmt.Errorf("deliverTxSub was cancelled (reason: %s)", reason)
		l.Logger.Error("Error on broadcastTxCommit", "err", err)
		return &ctypes.ResultBroadcastTxCommit{
			CheckTx:   *checkTxRes,
			DeliverTx: abci.ResponseDeliverTx{},
			Hash:      tx.Hash(),
		}, err
	case <-time.After(l.config.TimeoutBroadcastTxCommit):
		err = errors.New("timed out waiting for tx to be included in a block")
		l.Logger.Error("Error on broadcastTxCommit", "err", err)
		return &ctypes.ResultBroadcastTxCommit{
			CheckTx:   *checkTxRes,
			DeliverTx: abci.ResponseDeliverTx{},
			Hash:      tx.Hash(),
		}, err
	}
}

// BroadcastTxAsync returns right away, with no response. Does not wait for
// CheckTx nor DeliverTx results.
// More: https://docs.tendermint.com/master/rpc/#/Tx/broadcast_tx_async
func (l *Local) BroadcastTxAsync(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTx, error) {
	err := l.node.Mempool.CheckTx(tx, nil, mempool.TxInfo{Context: ctx})

	if err != nil {
		return nil, err
	}
	return &ctypes.ResultBroadcastTx{Hash: tx.Hash()}, nil
}

// BroadcastTxSync returns with the response from CheckTx. Does not wait for
// DeliverTx result.
// More: https://docs.tendermint.com/master/rpc/#/Tx/broadcast_tx_sync
func (l *Local) BroadcastTxSync(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTx, error) {
	resCh := make(chan *abci.Response, 1)
	err := l.node.Mempool.CheckTx(tx, func(res *abci.Response) {
		resCh <- res
	}, mempool.TxInfo{Context: ctx})
	if err != nil {
		return nil, err
	}
	res := <-resCh
	r := res.GetCheckTx()
	return &ctypes.ResultBroadcastTx{
		Code:      r.Code,
		Data:      r.Data,
		Log:       r.Log,
		Codespace: r.Codespace,
		Hash:      tx.Hash(),
	}, nil
}

func (l *Local) Subscribe(ctx context.Context, subscriber, query string, outCapacity ...int) (out <-chan ctypes.ResultEvent, err error) {
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

func (l *Local) Unsubscribe(ctx context.Context, subscriber, query string) error {
	q, err := tmquery.New(query)
	if err != nil {
		return fmt.Errorf("failed to parse query: %w", err)
	}
	return l.EventBus.Unsubscribe(ctx, subscriber, q)
}

func (l *Local) Genesis(ctx context.Context) (*ctypes.ResultGenesis, error) {
	// needs genesis provider
	panic("Genesis - not implemented!")
}

func (l *Local) BlockchainInfo(ctx context.Context, minHeight, maxHeight int64) (*ctypes.ResultBlockchainInfo, error) {
	// needs block store
	panic("BlockchainInfo - not implemented!")
}

func (l *Local) NetInfo(ctx context.Context) (*ctypes.ResultNetInfo, error) {
	// needs P2P layer
	panic("NetInfo - not implemented!")
}

func (l *Local) DumpConsensusState(ctx context.Context) (*ctypes.ResultDumpConsensusState, error) {
	// need consensus state
	panic("DumpConsensusState - not implemented!")
}

func (l *Local) ConsensusState(ctx context.Context) (*ctypes.ResultConsensusState, error) {
	// need consensus state
	panic("ConsensusState - not implemented!")
}

func (l *Local) ConsensusParams(ctx context.Context, height *int64) (*ctypes.ResultConsensusParams, error) {
	// needs state storage
	panic("ConsensusParams - not implemented!")
}

func (l *Local) Health(ctx context.Context) (*ctypes.ResultHealth, error) {
	return &ctypes.ResultHealth{}, nil
}

func (l *Local) Block(ctx context.Context, height *int64) (*ctypes.ResultBlock, error) {
	// needs block store
	panic("Block - not implemented!")
}

func (l *Local) BlockByHash(ctx context.Context, hash []byte) (*ctypes.ResultBlock, error) {
	// needs block store
	panic("BlockByHash - not implemented!")
}

func (l *Local) BlockResults(ctx context.Context, height *int64) (*ctypes.ResultBlockResults, error) {
	// needs block store
	panic("BlockResults - not implemented!")
}

func (l *Local) Commit(ctx context.Context, height *int64) (*ctypes.ResultCommit, error) {
	// needs block store
	panic("Commit - not implemented!")
}

func (l *Local) Validators(ctx context.Context, height *int64, page, perPage *int) (*ctypes.ResultValidators, error) {
	panic("Validators - not implemented!")
}

func (l *Local) Tx(ctx context.Context, hash []byte, prove bool) (*ctypes.ResultTx, error) {
	// needs block store, tx index (?)
	panic("Tx - not implemented!")
}

func (l *Local) TxSearch(ctx context.Context, query string, prove bool, page, perPage *int, orderBy string) (*ctypes.ResultTxSearch, error) {
	// needs block store
	panic("TxSearch - not implemented!")
}

func (l *Local) Status(ctx context.Context) (*ctypes.ResultStatus, error) {
	latest, err := l.node.Store.LoadBlock(l.node.Store.Height())
	if err != nil {
		// TODO(tzdybal): extract error
		return nil, fmt.Errorf("failed to find latest block: %w", err)
	}

	latestBlockHash := latest.Header.DataHash
	latestAppHash := latest.Header.AppHash
	latestHeight := latest.Header.Height
	latestBlockTimeNano := latest.Header.Time

	result := &ctypes.ResultStatus{
		// TODO(tzdybal): NodeInfo, ValidatorInfo
		SyncInfo: ctypes.SyncInfo{
			LatestBlockHash:   latestBlockHash[:],
			LatestAppHash:     latestAppHash[:],
			LatestBlockHeight: int64(latestHeight),
			LatestBlockTime:   time.Unix(0, int64(latestBlockTimeNano)),
			// TODO(tzdybal): add missing fields
			//EarliestBlockHash:   earliestBlockHash,
			//EarliestAppHash:     earliestAppHash,
			//EarliestBlockHeight: earliestBlockHeight,
			//EarliestBlockTime:   time.Unix(0, earliestBlockTimeNano),
			//CatchingUp:          env.ConsensusReactor.WaitSync(),
		},
	}
	return result, nil
}

func (l *Local) BroadcastEvidence(ctx context.Context, evidence types.Evidence) (*ctypes.ResultBroadcastEvidence, error) {
	// needs evidence pool?
	panic("BroadcastEvidence - not implemented!")
}

func (l *Local) UnconfirmedTxs(ctx context.Context, limit *int) (*ctypes.ResultUnconfirmedTxs, error) {
	// needs mempool
	panic("UnconfirmedTxs - not implemented!")
}

func (l *Local) NumUnconfirmedTxs(ctx context.Context) (*ctypes.ResultUnconfirmedTxs, error) {
	// needs mempool
	panic("NumUnconfirmedTxs - not implemented!")
}

func (l *Local) CheckTx(ctx context.Context, tx types.Tx) (*ctypes.ResultCheckTx, error) {
	res, err := l.mempool().CheckTxSync(ctx, abci.RequestCheckTx{Tx: tx})
	if err != nil {
		return nil, err
	}
	return &ctypes.ResultCheckTx{ResponseCheckTx: *res}, nil
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

func (l *Local) consensus() proxy.AppConnConsensus {
	return l.node.ProxyApp().Consensus()
}

func (l *Local) mempool() proxy.AppConnMempool {
	return l.node.ProxyApp().Mempool()
}

func (l *Local) query() proxy.AppConnQuery {
	return l.node.ProxyApp().Query()
}

func (l *Local) snapshot() proxy.AppConnSnapshot {
	return l.node.ProxyApp().Snapshot()
}
