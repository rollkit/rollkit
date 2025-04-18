package node

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/config"
	cmbytes "github.com/cometbft/cometbft/libs/bytes"
	cmpubsub "github.com/cometbft/cometbft/libs/pubsub"
	cmquery "github.com/cometbft/cometbft/libs/pubsub/query"
	corep2p "github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/proxy"
	rpcclient "github.com/cometbft/cometbft/rpc/client"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	cmtypes "github.com/cometbft/cometbft/types"
	"github.com/cometbft/cometbft/version"

	rconfig "github.com/rollkit/rollkit/config"
	"github.com/rollkit/rollkit/mempool"
	"github.com/rollkit/rollkit/types"
	abciconv "github.com/rollkit/rollkit/types/abci"
)

const (
	defaultPerPage = 30
	maxPerPage     = 100

	// TODO(tzdybal): make this configurable
	subscribeTimeout = 5 * time.Second
)

var (
	// ErrConsensusStateNotAvailable is returned because Rollkit doesn't use Tendermint consensus.
	ErrConsensusStateNotAvailable = errors.New("consensus state not available in Rollkit")
)

var _ rpcclient.Client = &FullClient{}

// FullClient implements tendermint RPC client interface.
//
// This is the type that is used in communication between cosmos-sdk app and Rollkit.
type FullClient struct {
	*cmtypes.EventBus
	config *config.RPCConfig
	node   *FullNode
}

// NewFullClient returns Client working with given node.
func NewFullClient(node *FullNode) *FullClient {
	return &FullClient{
		EventBus: node.EventBus(),
		config:   config.DefaultRPCConfig(),
		node:     node,
	}
}

// ABCIInfo returns basic information about application state.
func (c *FullClient) ABCIInfo(ctx context.Context) (*ctypes.ResultABCIInfo, error) {
	resInfo, err := c.appClient().Query().Info(ctx, proxy.RequestInfo)
	if err != nil {
		return nil, err
	}
	return &ctypes.ResultABCIInfo{Response: *resInfo}, nil
}

// ABCIQuery queries for data from application.
func (c *FullClient) ABCIQuery(ctx context.Context, path string, data cmbytes.HexBytes) (*ctypes.ResultABCIQuery, error) {
	return c.ABCIQueryWithOptions(ctx, path, data, rpcclient.DefaultABCIQueryOptions)
}

// ABCIQueryWithOptions queries for data from application.
func (c *FullClient) ABCIQueryWithOptions(ctx context.Context, path string, data cmbytes.HexBytes, opts rpcclient.ABCIQueryOptions) (*ctypes.ResultABCIQuery, error) {
	resQuery, err := c.appClient().Query().Query(ctx, &abci.RequestQuery{
		Path:   path,
		Data:   data,
		Height: opts.Height,
		Prove:  opts.Prove,
	})
	if err != nil {
		return nil, err
	}
	c.Logger.Debug("ABCIQuery", "path", path, "data", data, "result", resQuery)
	return &ctypes.ResultABCIQuery{Response: *resQuery}, nil
}

// BroadcastTxCommit returns with the responses from CheckTx and DeliverTx.
// More: https://docs.tendermint.com/master/rpc/#/Tx/broadcast_tx_commit
func (c *FullClient) BroadcastTxCommit(ctx context.Context, tx cmtypes.Tx) (*ctypes.ResultBroadcastTxCommit, error) {
	// This implementation corresponds to Tendermints implementation from rpc/core/mempool.go.
	// ctx.RemoteAddr godoc: If neither HTTPReq nor WSConn is set, an empty string is returned.
	// This code is a local client, so we can assume that subscriber is ""
	subscriber := "" //ctx.RemoteAddr()

	if c.EventBus.NumClients() >= c.config.MaxSubscriptionClients {
		return nil, fmt.Errorf("max_subscription_clients %d reached", c.config.MaxSubscriptionClients)
	} else if c.EventBus.NumClientSubscriptions(subscriber) >= c.config.MaxSubscriptionsPerClient {
		return nil, fmt.Errorf("max_subscriptions_per_client %d reached", c.config.MaxSubscriptionsPerClient)
	}

	// Subscribe to tx being committed in block.
	subCtx, cancel := context.WithTimeout(ctx, subscribeTimeout)
	defer cancel()
	q := cmtypes.EventQueryTxFor(tx)
	deliverTxSub, err := c.EventBus.Subscribe(subCtx, subscriber, q)
	if err != nil {
		err = fmt.Errorf("failed to subscribe to tx: %w", err)
		c.Logger.Error("Error on broadcast_tx_commit", "err", err)
		return nil, err
	}
	defer func() {
		if err := c.EventBus.Unsubscribe(ctx, subscriber, q); err != nil {
			c.Logger.Error("Error unsubscribing from eventBus", "err", err)
		}
	}()

	// add to mempool and wait for CheckTx result
	checkTxResCh := make(chan *abci.ResponseCheckTx, 1)
	err = c.node.Mempool.CheckTx(tx, func(res *abci.ResponseCheckTx) {
		select {
		case <-ctx.Done():
			return
		case checkTxResCh <- res:
		}
	}, mempool.TxInfo{})
	if err != nil {
		c.Logger.Error("Error on broadcastTxCommit", "err", err)
		return nil, fmt.Errorf("error on broadcastTxCommit: %w", err)
	}
	checkTxRes := <-checkTxResCh
	if checkTxRes.Code != abci.CodeTypeOK {
		return &ctypes.ResultBroadcastTxCommit{
			CheckTx:  *checkTxRes,
			TxResult: abci.ExecTxResult{},
			Hash:     tx.Hash(),
		}, nil
	}

	// broadcast tx
	err = c.node.p2pClient.GossipTx(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("tx added to local mempool but failure to broadcast: %w", err)
	}

	// Wait for the tx to be included in a block or timeout.
	select {
	case msg := <-deliverTxSub.Out(): // The tx was included in a block.
		deliverTxRes := msg.Data().(cmtypes.EventDataTx)
		return &ctypes.ResultBroadcastTxCommit{
			CheckTx:  *checkTxRes,
			TxResult: deliverTxRes.Result,
			Hash:     tx.Hash(),
			Height:   deliverTxRes.Height,
		}, nil
	case <-deliverTxSub.Canceled():
		var reason string
		if deliverTxSub.Err() == nil {
			reason = "Tendermint exited"
		} else {
			reason = deliverTxSub.Err().Error()
		}
		err = fmt.Errorf("deliverTxSub was cancelled (reason: %s)", reason)
		c.Logger.Error("Error on broadcastTxCommit", "err", err)
		return &ctypes.ResultBroadcastTxCommit{
			CheckTx:  *checkTxRes,
			TxResult: abci.ExecTxResult{},
			Hash:     tx.Hash(),
		}, err
	case <-time.After(c.config.TimeoutBroadcastTxCommit):
		err = errors.New("timed out waiting for tx to be included in a block")
		c.Logger.Error("Error on broadcastTxCommit", "err", err)
		return &ctypes.ResultBroadcastTxCommit{
			CheckTx:  *checkTxRes,
			TxResult: abci.ExecTxResult{},
			Hash:     tx.Hash(),
		}, err
	}
}

// BroadcastTxAsync returns right away, with no response. Does not wait for
// CheckTx nor DeliverTx results.
// More: https://docs.tendermint.com/master/rpc/#/Tx/broadcast_tx_async
func (c *FullClient) BroadcastTxAsync(ctx context.Context, tx cmtypes.Tx) (*ctypes.ResultBroadcastTx, error) {
	err := c.node.Mempool.CheckTx(tx, nil, mempool.TxInfo{})
	if err != nil {
		return nil, err
	}
	// gossipTx optimistically
	err = c.node.p2pClient.GossipTx(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("tx added to local mempool but failed to gossip: %w", err)
	}
	return &ctypes.ResultBroadcastTx{Hash: tx.Hash()}, nil
}

// BroadcastTxSync returns with the response from CheckTx. Does not wait for
// DeliverTx result.
// More: https://docs.tendermint.com/master/rpc/#/Tx/broadcast_tx_sync
func (c *FullClient) BroadcastTxSync(ctx context.Context, tx cmtypes.Tx) (*ctypes.ResultBroadcastTx, error) {
	resCh := make(chan *abci.ResponseCheckTx, 1)
	err := c.node.Mempool.CheckTx(tx, func(res *abci.ResponseCheckTx) {
		select {
		case <-ctx.Done():
			return
		case resCh <- res:
		}
	}, mempool.TxInfo{})
	if err != nil {
		return nil, err
	}
	res := <-resCh

	// gossip the transaction if it's in the mempool.
	// Note: we have to do this here because, unlike the tendermint mempool reactor, there
	// is no routine that gossips transactions after they enter the pool
	if res.Code == abci.CodeTypeOK {
		err = c.node.p2pClient.GossipTx(ctx, tx)
		if err != nil {
			// the transaction must be removed from the mempool if it cannot be gossiped.
			// if this does not occur, then the user will not be able to try again using
			// this node, as the CheckTx call above will return an error indicating that
			// the tx is already in the mempool
			_ = c.node.Mempool.RemoveTxByKey(tx.Key())
			return nil, fmt.Errorf("failed to gossip tx: %w", err)
		}
	}

	return &ctypes.ResultBroadcastTx{
		Code:      res.Code,
		Data:      res.Data,
		Log:       res.Log,
		Codespace: res.Codespace,
		Hash:      tx.Hash(),
	}, nil
}

// Subscribe subscribe given subscriber to a query.
func (c *FullClient) Subscribe(ctx context.Context, subscriber, query string, outCapacity ...int) (out <-chan ctypes.ResultEvent, err error) {
	q, err := cmquery.New(query)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query: %w", err)
	}

	outCap := 1
	if len(outCapacity) > 0 {
		outCap = outCapacity[0]
	}

	var sub cmtypes.Subscription
	if outCap > 0 {
		sub, err = c.EventBus.Subscribe(ctx, subscriber, q, outCap)
	} else {
		sub, err = c.EventBus.SubscribeUnbuffered(ctx, subscriber, q)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe: %w", err)
	}

	outc := make(chan ctypes.ResultEvent, outCap)
	go c.eventsRoutine(sub, subscriber, q, outc)

	return outc, nil
}

// Unsubscribe unsubscribes given subscriber from a query.
func (c *FullClient) Unsubscribe(ctx context.Context, subscriber, query string) error {
	q, err := cmquery.New(query)
	if err != nil {
		return fmt.Errorf("failed to parse query: %w", err)
	}
	return c.EventBus.Unsubscribe(ctx, subscriber, q)
}

// Genesis returns entire genesis.
func (c *FullClient) Genesis(_ context.Context) (*ctypes.ResultGenesis, error) {
	return &ctypes.ResultGenesis{Genesis: c.node.GetGenesis()}, nil
}

// GenesisChunked returns given chunk of genesis.
func (c *FullClient) GenesisChunked(context context.Context, id uint) (*ctypes.ResultGenesisChunk, error) {
	genChunks, err := c.node.GetGenesisChunks()
	if err != nil {
		return nil, fmt.Errorf("error while creating chunks of the genesis document: %w", err)
	}
	if genChunks == nil {
		return nil, errors.New("service configuration error, genesis chunks are not initialized")
	}

	chunkLen := len(genChunks)
	if chunkLen == 0 {
		return nil, errors.New("service configuration error, there are no chunks")
	}

	if int(id) > chunkLen-1 {
		return nil, fmt.Errorf("there are %d chunks, %d is invalid", chunkLen-1, id)
	}

	return &ctypes.ResultGenesisChunk{
		TotalChunks: chunkLen,
		ChunkNumber: int(id),
		Data:        genChunks[id],
	}, nil
}

// BlockchainInfo returns ABCI block meta information for given height range.
func (c *FullClient) BlockchainInfo(ctx context.Context, minHeight, maxHeight int64) (*ctypes.ResultBlockchainInfo, error) {
	const limit int64 = 20

	// Currently blocks are not pruned and are synced linearly so the base height is 0
	minHeight, maxHeight, err := filterMinMax(
		0,
		int64(c.node.Store.Height()),
		minHeight,
		maxHeight,
		limit)
	if err != nil {
		return nil, err
	}
	c.Logger.Debug("BlockchainInfo", "maxHeight", maxHeight, "minHeight", minHeight)

	blocks := make([]*cmtypes.BlockMeta, 0, maxHeight-minHeight+1)
	for height := maxHeight; height >= minHeight; height-- {
		header, data, err := c.node.Store.GetBlockData(ctx, uint64(height))
		if err != nil {
			return nil, err
		}
		if header != nil && data != nil {
			cmblockmeta, err := abciconv.ToABCIBlockMeta(header, data)
			if err != nil {
				return nil, err
			}
			blocks = append(blocks, cmblockmeta)
		}
	}

	return &ctypes.ResultBlockchainInfo{
		LastHeight: int64(c.node.Store.Height()),
		BlockMetas: blocks,
	}, nil

}

// NetInfo returns basic information about client P2P connections.
func (c *FullClient) NetInfo(ctx context.Context) (*ctypes.ResultNetInfo, error) {
	res := ctypes.ResultNetInfo{
		Listening: true,
	}
	for _, ma := range c.node.p2pClient.Addrs() {
		res.Listeners = append(res.Listeners, ma.String())
	}
	peers := c.node.p2pClient.Peers()
	res.NPeers = len(peers)
	for _, peer := range peers {
		res.Peers = append(res.Peers, ctypes.Peer{
			NodeInfo:         peer.NodeInfo,
			IsOutbound:       peer.IsOutbound,
			ConnectionStatus: peer.ConnectionStatus,
			RemoteIP:         peer.RemoteIP,
		})
	}

	return &res, nil
}

// DumpConsensusState always returns error as there is no consensus state in Rollkit.
func (c *FullClient) DumpConsensusState(ctx context.Context) (*ctypes.ResultDumpConsensusState, error) {
	return nil, ErrConsensusStateNotAvailable
}

// ConsensusState always returns error as there is no consensus state in Rollkit.
func (c *FullClient) ConsensusState(ctx context.Context) (*ctypes.ResultConsensusState, error) {
	return nil, ErrConsensusStateNotAvailable
}

// ConsensusParams returns consensus params at given height.
func (c *FullClient) ConsensusParams(ctx context.Context, height *int64) (*ctypes.ResultConsensusParams, error) {
	state, err := c.node.Store.GetState(ctx)
	if err != nil {
		return nil, err
	}
	params := state.ConsensusParams
	return &ctypes.ResultConsensusParams{
		BlockHeight: int64(c.normalizeHeight(height)),
		ConsensusParams: cmtypes.ConsensusParams{
			Block: cmtypes.BlockParams{
				MaxBytes: params.Block.MaxBytes,
				MaxGas:   params.Block.MaxGas,
			},
			Evidence: cmtypes.EvidenceParams{
				MaxAgeNumBlocks: params.Evidence.MaxAgeNumBlocks,
				MaxAgeDuration:  params.Evidence.MaxAgeDuration,
				MaxBytes:        params.Evidence.MaxBytes,
			},
			Validator: cmtypes.ValidatorParams{
				PubKeyTypes: params.Validator.PubKeyTypes,
			},
			Version: cmtypes.VersionParams{
				App: params.Version.App,
			},
		},
	}, nil
}

// Health endpoint returns empty value. It can be used to monitor service availability.
func (c *FullClient) Health(ctx context.Context) (*ctypes.ResultHealth, error) {
	return &ctypes.ResultHealth{}, nil
}

// Block method returns BlockID and block itself for given height.
//
// If height is nil, it returns information about last known block.
// If height is a valid block tag, it returns information about the tagged
// block, see BlockNumber for details.
func (c *FullClient) Block(ctx context.Context, height *int64) (*ctypes.ResultBlock, error) {
	var heightValue uint64

	switch {
	// block tag = included
	case height != nil && *height == -1:
		heightValue = c.node.blockManager.GetDAIncludedHeight()
	default:
		heightValue = c.normalizeHeight(height)
	}
	header, data, err := c.node.Store.GetBlockData(ctx, heightValue)
	if err != nil {
		return nil, err
	}

	hash := header.Hash()
	abciBlock, err := abciconv.ToABCIBlock(header, data)
	if err != nil {
		return nil, err
	}
	return &ctypes.ResultBlock{
		BlockID: cmtypes.BlockID{
			Hash: cmbytes.HexBytes(hash),
			PartSetHeader: cmtypes.PartSetHeader{
				Total: 0,
				Hash:  nil,
			},
		},
		Block: abciBlock,
	}, nil
}

// BlockByHash returns BlockID and block itself for given hash.
func (c *FullClient) BlockByHash(ctx context.Context, hash []byte) (*ctypes.ResultBlock, error) {
	header, data, err := c.node.Store.GetBlockByHash(ctx, hash)
	if err != nil {
		return nil, err
	}

	abciBlock, err := abciconv.ToABCIBlock(header, data)
	if err != nil {
		return nil, err
	}
	return &ctypes.ResultBlock{
		BlockID: cmtypes.BlockID{
			Hash: hash,
			PartSetHeader: cmtypes.PartSetHeader{
				Total: 0,
				Hash:  nil,
			},
		},
		Block: abciBlock,
	}, nil
}

// BlockResults returns information about transactions, events and updates of validator set and consensus params.
func (c *FullClient) BlockResults(ctx context.Context, height *int64) (*ctypes.ResultBlockResults, error) {
	var h uint64
	if height == nil {
		h = c.node.Store.Height()
	} else {
		h = uint64(*height)
	}
	header, _, err := c.node.Store.GetBlockData(ctx, h)
	if err != nil {
		return nil, err
	}
	resp, err := c.node.Store.GetBlockResponses(ctx, h)
	if err != nil {
		return nil, err
	}

	return &ctypes.ResultBlockResults{
		Height:                int64(h),
		TxsResults:            resp.TxResults,
		FinalizeBlockEvents:   resp.Events,
		ValidatorUpdates:      resp.ValidatorUpdates,
		ConsensusParamUpdates: resp.ConsensusParamUpdates,
		AppHash:               header.Header.AppHash,
	}, nil
}

// Commit returns signed header (aka commit) at given height.
func (c *FullClient) Commit(ctx context.Context, height *int64) (*ctypes.ResultCommit, error) {
	heightValue := c.normalizeHeight(height)
	header, data, err := c.node.Store.GetBlockData(ctx, heightValue)
	if err != nil {
		return nil, err
	}

	// we should have a single validator
	if len(header.Validators.Validators) == 0 {
		return nil, errors.New("empty validator set found in block")
	}

	val := header.Validators.Validators[0].Address
	commit := types.GetABCICommit(heightValue, header.Hash(), val, header.Time(), header.Signature)

	block, err := abciconv.ToABCIBlock(header, data)
	if err != nil {
		return nil, err
	}

	return ctypes.NewResultCommit(&block.Header, commit, true), nil
}

// Validators returns paginated list of validators at given height.
func (c *FullClient) Validators(ctx context.Context, heightPtr *int64, pagePtr, perPagePtr *int) (*ctypes.ResultValidators, error) {
	height := c.normalizeHeight(heightPtr)
	genesisValidators := c.node.GetGenesis().Validators

	if len(genesisValidators) != 1 {
		return nil, errors.New("there should be exactly one validator in genesis")
	}
	// Since it's a centralized sequencer
	// changed behavior to get this from genesis
	genesisValidator := genesisValidators[0]
	validator := cmtypes.Validator{
		Address:          genesisValidator.Address,
		PubKey:           genesisValidator.PubKey,
		VotingPower:      int64(1),
		ProposerPriority: int64(1),
	}

	return &ctypes.ResultValidators{
		BlockHeight: int64(height),
		Validators: []*cmtypes.Validator{
			&validator,
		},
		Count: 1,
		Total: 1,
	}, nil
}

// Tx returns detailed information about transaction identified by its hash.
func (c *FullClient) Tx(ctx context.Context, hash []byte, prove bool) (*ctypes.ResultTx, error) {
	res, err := c.node.TxIndexer.Get(hash)
	if err != nil {
		return nil, err
	}

	if res == nil {
		return nil, fmt.Errorf("tx (%X) not found", hash)
	}

	height := res.Height
	index := res.Index

	var proof cmtypes.TxProof
	if prove {
		_, data, _ := c.node.Store.GetBlockData(ctx, uint64(height))
		blockProof := data.Txs.Proof(int(index)) // XXX: overflow on 32-bit machines
		proof = cmtypes.TxProof{
			RootHash: blockProof.RootHash,
			Data:     cmtypes.Tx(blockProof.Data),
			Proof:    blockProof.Proof,
		}
	}

	return &ctypes.ResultTx{
		Hash:     hash,
		Height:   height,
		Index:    index,
		TxResult: res.Result,
		Tx:       res.Tx,
		Proof:    proof,
	}, nil
}

// TxSearch returns detailed information about transactions matching query.
func (c *FullClient) TxSearch(ctx context.Context, query string, prove bool, pagePtr, perPagePtr *int, orderBy string) (*ctypes.ResultTxSearch, error) {
	q, err := cmquery.New(query)
	if err != nil {
		return nil, err
	}

	results, err := c.node.TxIndexer.Search(ctx, q)
	if err != nil {
		return nil, err
	}

	// sort results (must be done before pagination)
	switch orderBy {
	case "desc":
		sort.Slice(results, func(i, j int) bool {
			if results[i].Height == results[j].Height {
				return results[i].Index > results[j].Index
			}
			return results[i].Height > results[j].Height
		})
	case "asc", "":
		sort.Slice(results, func(i, j int) bool {
			if results[i].Height == results[j].Height {
				return results[i].Index < results[j].Index
			}
			return results[i].Height < results[j].Height
		})
	default:
		return nil, errors.New("expected order_by to be either `asc` or `desc` or empty")
	}

	// paginate results
	totalCount := len(results)
	perPage := validatePerPage(perPagePtr)

	page, err := validatePage(pagePtr, perPage, totalCount)
	if err != nil {
		return nil, err
	}

	skipCount := validateSkipCount(page, perPage)
	pageSize := min(perPage, totalCount-skipCount)

	apiResults := make([]*ctypes.ResultTx, 0, pageSize)
	for i := skipCount; i < skipCount+pageSize; i++ {
		r := results[i]

		var proof cmtypes.TxProof
		/*if prove {
			block := nil                               //env.BlockStore.GetBlock(r.Height)
			proof = block.Data.Txs.Proof(int(r.Index)) // XXX: overflow on 32-bit machines
		}*/

		apiResults = append(apiResults, &ctypes.ResultTx{
			Hash:     cmtypes.Tx(r.Tx).Hash(),
			Height:   r.Height,
			Index:    r.Index,
			TxResult: r.Result,
			Tx:       r.Tx,
			Proof:    proof,
		})
	}

	return &ctypes.ResultTxSearch{Txs: apiResults, TotalCount: totalCount}, nil
}

// BlockSearch defines a method to search for a paginated set of blocks by
// BeginBlock and EndBlock event search criteria.
func (c *FullClient) BlockSearch(ctx context.Context, query string, page, perPage *int, orderBy string) (*ctypes.ResultBlockSearch, error) {
	q, err := cmquery.New(query)
	if err != nil {
		return nil, err
	}

	results, err := c.node.BlockIndexer.Search(ctx, q)
	if err != nil {
		return nil, err
	}

	// Sort the results
	switch orderBy {
	case "desc":
		sort.Slice(results, func(i, j int) bool {
			return results[i] > results[j]
		})

	case "asc", "":
		sort.Slice(results, func(i, j int) bool {
			return results[i] < results[j]
		})
	default:
		return nil, errors.New("expected order_by to be either `asc` or `desc` or empty")
	}

	// Paginate
	totalCount := len(results)
	perPageVal := validatePerPage(perPage)

	pageVal, err := validatePage(page, perPageVal, totalCount)
	if err != nil {
		return nil, err
	}

	skipCount := validateSkipCount(pageVal, perPageVal)
	pageSize := min(perPageVal, totalCount-skipCount)

	// Fetch the blocks
	blocks := make([]*ctypes.ResultBlock, 0, pageSize)
	for i := skipCount; i < skipCount+pageSize; i++ {
		header, data, err := c.node.Store.GetBlockData(ctx, uint64(results[i]))
		if err != nil {
			return nil, err
		}
		block, err := abciconv.ToABCIBlock(header, data)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, &ctypes.ResultBlock{
			Block: block,
			BlockID: cmtypes.BlockID{
				Hash: block.Hash(),
			},
		})
	}

	return &ctypes.ResultBlockSearch{Blocks: blocks, TotalCount: totalCount}, nil
}

// Status returns detailed information about current status of the node.
func (c *FullClient) Status(ctx context.Context) (*ctypes.ResultStatus, error) {
	var (
		latestBlockHash cmbytes.HexBytes
		latestAppHash   cmbytes.HexBytes
		latestBlockTime time.Time

		latestHeight = c.node.Store.Height()
	)

	if latestHeight != 0 {
		header, _, err := c.node.Store.GetBlockData(ctx, latestHeight)
		if err != nil {
			return nil, fmt.Errorf("failed to find latest block: %w", err)
		}
		latestBlockHash = cmbytes.HexBytes(header.DataHash)
		latestAppHash = cmbytes.HexBytes(header.AppHash)
		latestBlockTime = header.Time()
	}

	initialHeader, _, err := c.node.Store.GetBlockData(ctx, uint64(c.node.GetGenesis().InitialHeight))
	if err != nil {
		return nil, fmt.Errorf("failed to find earliest block: %w", err)
	}

	genesisValidators := c.node.GetGenesis().Validators

	if len(genesisValidators) != 1 {
		return nil, errors.New("there should be exactly one validator in genesis")
	}

	// Changed behavior to get this from genesis
	genesisValidator := genesisValidators[0]
	validator := cmtypes.Validator{
		Address:          genesisValidator.Address,
		PubKey:           genesisValidator.PubKey,
		VotingPower:      int64(1),
		ProposerPriority: int64(1),
	}

	state, err := c.node.Store.GetState(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load the last saved state: %w", err)
	}
	defaultProtocolVersion := corep2p.NewProtocolVersion(
		version.P2PProtocol,
		state.Version.Consensus.Block,
		state.Version.Consensus.App,
	)
	id, addr, network, err := c.node.p2pClient.Info()
	if err != nil {
		return nil, fmt.Errorf("failed to load node p2p2 info: %w", err)
	}
	txIndexerStatus := "on"

	result := &ctypes.ResultStatus{
		NodeInfo: corep2p.DefaultNodeInfo{
			ProtocolVersion: defaultProtocolVersion,
			DefaultNodeID:   id,
			ListenAddr:      addr,
			Network:         network,
			Version:         rconfig.Version,
			Moniker:         config.DefaultBaseConfig().Moniker,
			Other: corep2p.DefaultNodeInfoOther{
				TxIndex:    txIndexerStatus,
				RPCAddress: c.config.ListenAddress,
			},
		},
		SyncInfo: ctypes.SyncInfo{
			LatestBlockHash:     latestBlockHash,
			LatestAppHash:       latestAppHash,
			LatestBlockHeight:   int64(latestHeight),
			LatestBlockTime:     latestBlockTime,
			EarliestBlockHash:   cmbytes.HexBytes(initialHeader.DataHash),
			EarliestAppHash:     cmbytes.HexBytes(initialHeader.AppHash),
			EarliestBlockHeight: int64(initialHeader.Height()),
			EarliestBlockTime:   initialHeader.Time(),
			CatchingUp:          false, // hard-code this to "false" to pass Go IBC relayer's legacy encoding check
		},
		ValidatorInfo: ctypes.ValidatorInfo{
			Address:     validator.Address,
			PubKey:      validator.PubKey,
			VotingPower: validator.VotingPower,
		},
	}
	return result, nil
}

// BroadcastEvidence is not yet implemented.
func (c *FullClient) BroadcastEvidence(ctx context.Context, evidence cmtypes.Evidence) (*ctypes.ResultBroadcastEvidence, error) {
	return &ctypes.ResultBroadcastEvidence{
		Hash: evidence.Hash(),
	}, nil
}

// NumUnconfirmedTxs returns information about transactions in mempool.
func (c *FullClient) NumUnconfirmedTxs(ctx context.Context) (*ctypes.ResultUnconfirmedTxs, error) {
	return &ctypes.ResultUnconfirmedTxs{
		Count:      c.node.Mempool.Size(),
		Total:      c.node.Mempool.Size(),
		TotalBytes: c.node.Mempool.SizeBytes(),
	}, nil

}

// UnconfirmedTxs returns transactions in mempool.
func (c *FullClient) UnconfirmedTxs(ctx context.Context, limitPtr *int) (*ctypes.ResultUnconfirmedTxs, error) {
	// reuse per_page validator
	limit := validatePerPage(limitPtr)

	txs := c.node.Mempool.ReapMaxTxs(limit)
	return &ctypes.ResultUnconfirmedTxs{
		Count:      len(txs),
		Total:      c.node.Mempool.Size(),
		TotalBytes: c.node.Mempool.SizeBytes(),
		Txs:        txs}, nil
}

// CheckTx executes a new transaction against the application to determine its validity.
//
// If valid, the tx is automatically added to the mempool.
func (c *FullClient) CheckTx(ctx context.Context, tx cmtypes.Tx) (*ctypes.ResultCheckTx, error) {
	res, err := c.appClient().Mempool().CheckTx(ctx, &abci.RequestCheckTx{Tx: tx})
	if err != nil {
		return nil, err
	}
	return &ctypes.ResultCheckTx{ResponseCheckTx: *res}, nil
}

// Header returns a cometbft ResultsHeader for the FullClient
func (c *FullClient) Header(ctx context.Context, heightPtr *int64) (*ctypes.ResultHeader, error) {
	height := c.normalizeHeight(heightPtr)
	blockMeta := c.getBlockMeta(ctx, height)
	if blockMeta == nil {
		return nil, fmt.Errorf("block at height %d not found", height)
	}
	return &ctypes.ResultHeader{Header: &blockMeta.Header}, nil
}

// HeaderByHash loads the block for the provided hash and returns the header
func (c *FullClient) HeaderByHash(ctx context.Context, hash cmbytes.HexBytes) (*ctypes.ResultHeader, error) {
	// N.B. The hash parameter is HexBytes so that the reflective parameter
	// decoding logic in the HTTP service will correctly translate from JSON.
	// See https://github.com/cometbft/cometbft/issues/6802 for context.

	header, data, err := c.node.Store.GetBlockByHash(ctx, types.Hash(hash))
	if err != nil {
		return nil, err
	}

	blockMeta, err := abciconv.ToABCIBlockMeta(header, data)
	if err != nil {
		return nil, err
	}

	if blockMeta == nil {
		return &ctypes.ResultHeader{}, nil
	}

	return &ctypes.ResultHeader{Header: &blockMeta.Header}, nil
}

func (c *FullClient) eventsRoutine(sub cmtypes.Subscription, subscriber string, q cmpubsub.Query, outc chan<- ctypes.ResultEvent) {
	defer close(outc)
	for {
		select {
		case msg := <-sub.Out():
			result := ctypes.ResultEvent{Query: q.String(), Data: msg.Data(), Events: msg.Events()}
			select {
			case outc <- result:
			default:
				// The default case can happen if the outc chan
				// is full or if it was initialized incorrectly
				// with a capacity of 0. Since this function has
				// no control over re-initializing the outc
				// chan, we do not block on a capacity of 0.
				full := cap(outc) != 0
				c.Logger.Error("wanted to publish ResultEvent, but out channel is full:", full, "result:", result, "query:", result.Query)
			}
		case <-sub.Canceled():
			if sub != nil && errors.Is(sub.Err(), cmpubsub.ErrUnsubscribed) {
				return
			}

			c.Logger.Error("subscription was cancelled, resubscribing...", "err", sub.Err(), "query", q.String())
			sub = c.resubscribe(subscriber, q)
			if sub == nil { // client was stopped
				return
			}
		case <-c.Quit():
			return
		}
	}
}

// Try to resubscribe with exponential backoff.
func (c *FullClient) resubscribe(subscriber string, q cmpubsub.Query) cmtypes.Subscription {
	attempts := 0
	for {
		if !c.IsRunning() {
			return nil
		}

		sub, err := c.EventBus.Subscribe(c.node.ctx, subscriber, q)
		if err == nil {
			return sub
		}

		attempts++
		// 10ms -> 20ms -> 40ms
		time.Sleep((10 << uint(attempts)) * time.Millisecond)
	}
}

func (c *FullClient) appClient() proxy.AppConns {
	return c.node.AppClient()
}

func (c *FullClient) normalizeHeight(height *int64) uint64 {
	var heightValue uint64
	if height == nil {
		heightValue = c.node.Store.Height()
	} else {
		heightValue = uint64(*height)
	}

	return heightValue
}

func (rpc *FullClient) getBlockMeta(ctx context.Context, n uint64) *cmtypes.BlockMeta {
	header, data, err := rpc.node.Store.GetBlockData(ctx, n)
	if err != nil {
		return nil
	}
	bmeta, err := abciconv.ToABCIBlockMeta(header, data)
	if err != nil {
		return nil
	}

	return bmeta
}

func validatePerPage(perPagePtr *int) int {
	if perPagePtr == nil { // no per_page parameter
		return defaultPerPage
	}

	perPage := *perPagePtr
	if perPage < 1 {
		return defaultPerPage
	} else if perPage > maxPerPage {
		return maxPerPage
	}
	return perPage
}

func validatePage(pagePtr *int, perPage, totalCount int) (int, error) {
	if perPage < 1 {
		panic(fmt.Sprintf("zero or negative perPage: %d", perPage))
	}

	if pagePtr == nil { // no page parameter
		return 1, nil
	}

	pages := ((totalCount - 1) / perPage) + 1
	if pages == 0 {
		pages = 1 // one page (even if it's empty)
	}
	page := *pagePtr
	if page <= 0 || page > pages {
		return 1, fmt.Errorf("page should be within [1, %d] range, given %d", pages, page)
	}

	return page, nil
}

func validateSkipCount(page, perPage int) int {
	skipCount := (page - 1) * perPage
	if skipCount < 0 {
		return 0
	}

	return skipCount
}

func filterMinMax(base, height, mini, maxi, limit int64) (int64, int64, error) {
	// filter negatives
	if mini < 0 || maxi < 0 {
		return mini, maxi, errors.New("height must be greater than zero")
	}

	// adjust for default values
	if mini == 0 {
		mini = 1
	}
	if maxi == 0 {
		maxi = height
	}

	// limit max to the height
	maxi = min(height, maxi)

	// limit min to the base
	mini = max(base, mini)

	// limit min to within `limit` of max
	// so the total number of blocks returned will be `limit`
	mini = max(mini, maxi-limit+1)

	if mini > maxi {
		return mini, maxi, fmt.Errorf("%w: min height %d can't be greater than max height %d",
			errors.New("invalid request"), mini, maxi)
	}
	return mini, maxi, nil
}
