package node

import (
	"context"

	cmbytes "github.com/cometbft/cometbft/libs/bytes"
	rpcclient "github.com/cometbft/cometbft/rpc/client"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/cometbft/cometbft/types"
)

var _ rpcclient.Client = &LightClient{}

type LightClient struct {
	types.EventBus
	node *LightNode
}

func NewLightClient(node *LightNode) *LightClient {
	return &LightClient{
		node: node,
	}
}

// ABCIInfo returns basic information about application state.
func (c *LightClient) ABCIInfo(ctx context.Context) (*ctypes.ResultABCIInfo, error) {
	panic("Not implemented")
}

// ABCIQuery queries for data from application.
func (c *LightClient) ABCIQuery(ctx context.Context, path string, data cmbytes.HexBytes) (*ctypes.ResultABCIQuery, error) {
	panic("Not implemented")
}

// ABCIQueryWithOptions queries for data from application.
func (c *LightClient) ABCIQueryWithOptions(ctx context.Context, path string, data cmbytes.HexBytes, opts rpcclient.ABCIQueryOptions) (*ctypes.ResultABCIQuery, error) {
	panic("Not implemented")
}

// BroadcastTxCommit returns with the responses from CheckTx and DeliverTx.
// More: https://docs.tendermint.com/master/rpc/#/Tx/broadcast_tx_commit
func (c *LightClient) BroadcastTxCommit(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTxCommit, error) {
	panic("Not implemented")
}

// BroadcastTxAsync returns right away, with no response. Does not wait for
// CheckTx nor DeliverTx results.
// More: https://docs.tendermint.com/master/rpc/#/Tx/broadcast_tx_async
func (c *LightClient) BroadcastTxAsync(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTx, error) {
	panic("Not implemented")
}

// BroadcastTxSync returns with the response from CheckTx. Does not wait for
// DeliverTx result.
// More: https://docs.tendermint.com/master/rpc/#/Tx/broadcast_tx_sync
func (c *LightClient) BroadcastTxSync(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTx, error) {
	panic("Not implemented")
}

// Subscribe subscribe given subscriber to a query.
func (c *LightClient) Subscribe(ctx context.Context, subscriber, query string, outCapacity ...int) (out <-chan ctypes.ResultEvent, err error) {
	panic("Not implemented")
}

// Unsubscribe unsubscribes given subscriber from a query.
func (c *LightClient) Unsubscribe(ctx context.Context, subscriber, query string) error {
	panic("Not implemented")
}

// Genesis returns entire genesis.
func (c *LightClient) Genesis(_ context.Context) (*ctypes.ResultGenesis, error) {
	panic("Not implemented")
}

// GenesisChunked returns given chunk of genesis.
func (c *LightClient) GenesisChunked(context context.Context, id uint) (*ctypes.ResultGenesisChunk, error) {
	panic("Not implemented")
}

// BlockchainInfo returns ABCI block meta information for given height range.
func (c *LightClient) BlockchainInfo(ctx context.Context, minHeight, maxHeight int64) (*ctypes.ResultBlockchainInfo, error) {
	panic("Not implemented")
}

// NetInfo returns basic information about client P2P connections.
func (c *LightClient) NetInfo(ctx context.Context) (*ctypes.ResultNetInfo, error) {
	panic("Not implemented")
}

// DumpConsensusState always returns error as there is no consensus state in Rollkit.
func (c *LightClient) DumpConsensusState(ctx context.Context) (*ctypes.ResultDumpConsensusState, error) {
	panic("Not implemented")
}

// ConsensusState always returns error as there is no consensus state in Rollkit.
func (c *LightClient) ConsensusState(ctx context.Context) (*ctypes.ResultConsensusState, error) {
	panic("Not implemented")
}

// ConsensusParams returns consensus params at given height.
//
// Currently, consensus params changes are not supported and this method returns params as defined in genesis.
func (c *LightClient) ConsensusParams(ctx context.Context, height *int64) (*ctypes.ResultConsensusParams, error) {
	panic("Not implemented")
}

// Health endpoint returns empty value. It can be used to monitor service availability.
func (c *LightClient) Health(ctx context.Context) (*ctypes.ResultHealth, error) {
	panic("Not implemented")
}

// Block method returns BlockID and block itself for given height.
//
// If height is nil, it returns information about last known block.
func (c *LightClient) Block(ctx context.Context, height *int64) (*ctypes.ResultBlock, error) {
	panic("Not implemented")
}

// BlockByHash returns BlockID and block itself for given hash.
func (c *LightClient) BlockByHash(ctx context.Context, hash []byte) (*ctypes.ResultBlock, error) {
	panic("Not implemented")
}

// BlockResults returns information about transactions, events and updates of validator set and consensus params.
func (c *LightClient) BlockResults(ctx context.Context, height *int64) (*ctypes.ResultBlockResults, error) {
	panic("Not implemented")
}

// Commit returns signed header (aka commit) at given height.
func (c *LightClient) Commit(ctx context.Context, height *int64) (*ctypes.ResultCommit, error) {
	panic("Not implemented")
}

// Validators returns paginated list of validators at given height.
func (c *LightClient) Validators(ctx context.Context, heightPtr *int64, pagePtr, perPagePtr *int) (*ctypes.ResultValidators, error) {
	panic("Not implemented")
}

// Tx returns detailed information about transaction identified by its hash.
func (c *LightClient) Tx(ctx context.Context, hash []byte, prove bool) (*ctypes.ResultTx, error) {
	panic("Not implemented")
}

// TxSearch returns detailed information about transactions matching query.
func (c *LightClient) TxSearch(ctx context.Context, query string, prove bool, pagePtr, perPagePtr *int, orderBy string) (*ctypes.ResultTxSearch, error) {
	panic("Not implemented")
}

// BlockSearch defines a method to search for a paginated set of blocks by
// BeginBlock and EndBlock event search criteria.
func (c *LightClient) BlockSearch(ctx context.Context, query string, page, perPage *int, orderBy string) (*ctypes.ResultBlockSearch, error) {
	panic("Not implemented")
}

// Status returns detailed information about current status of the node.
func (c *LightClient) Status(ctx context.Context) (*ctypes.ResultStatus, error) {
	panic("Not implemented")
}

// BroadcastEvidence is not yet implemented.
func (c *LightClient) BroadcastEvidence(ctx context.Context, evidence types.Evidence) (*ctypes.ResultBroadcastEvidence, error) {
	panic("Not implemented")
}

// NumUnconfirmedTxs returns information about transactions in mempool.
func (c *LightClient) NumUnconfirmedTxs(ctx context.Context) (*ctypes.ResultUnconfirmedTxs, error) {
	panic("Not implemented")
}

// UnconfirmedTxs returns transactions in mempool.
func (c *LightClient) UnconfirmedTxs(ctx context.Context, limitPtr *int) (*ctypes.ResultUnconfirmedTxs, error) {
	panic("Not implemented")
}

// CheckTx executes a new transaction against the application to determine its validity.
//
// If valid, the tx is automatically added to the mempool.
func (c *LightClient) CheckTx(ctx context.Context, tx types.Tx) (*ctypes.ResultCheckTx, error) {
	panic("Not implemented")
}

func (c *LightClient) Header(ctx context.Context, height *int64) (*ctypes.ResultHeader, error) {
	panic("Not implemented")
}

func (c *LightClient) HeaderByHash(ctx context.Context, hash cmbytes.HexBytes) (*ctypes.ResultHeader, error) {
	panic("Not implemented")
}
