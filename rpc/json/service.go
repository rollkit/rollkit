package json

import (
	"errors"
	"net/http"
	"reflect"

	"github.com/gorilla/rpc/v2/json2"
	rpcclient "github.com/tendermint/tendermint/rpc/client"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"

	"github.com/celestiaorg/optimint/log"
	"github.com/celestiaorg/optimint/rpc/client"
)

func GetHttpHandler(l *client.Client, logger log.Logger) (http.Handler, error) {
	return newHandler(newService(l), json2.NewCodec(), logger), nil
}

type method struct {
	m          reflect.Value
	argsType   reflect.Type
	returnType reflect.Type
}

func newMethod(m interface{}) *method {
	mType := reflect.TypeOf(m)

	return &method{
		m:          reflect.ValueOf(m),
		argsType:   mType.In(1).Elem(),
		returnType: mType.Out(0).Elem(),
	}
}

type service struct {
	client  *client.Client
	methods map[string]*method
}

func newService(c *client.Client) *service {
	s := service{
		client: c,
	}
	s.methods = map[string]*method{
		"subscribe":            newMethod(s.Subscribe),
		"unsubscribe":          newMethod(s.Unsubscribe),
		"unsubscribe_all":      newMethod(s.UnsubscribeAll),
		"health":               newMethod(s.Health),
		"status":               newMethod(s.Status),
		"net_info":             newMethod(s.NetInfo),
		"blockchain":           newMethod(s.BlockchainInfo),
		"genesis":              newMethod(s.Genesis),
		"genesis_chunked":      newMethod(s.GenesisChunked),
		"block":                newMethod(s.Block),
		"block_by_hash":        newMethod(s.BlockByHash),
		"block_results":        newMethod(s.BlockResults),
		"commit":               newMethod(s.Commit),
		"check_tx":             newMethod(s.CheckTx),
		"tx":                   newMethod(s.Tx),
		"tx_search":            newMethod(s.TxSearch),
		"block_search":         newMethod(s.BlockSearch),
		"validators":           newMethod(s.Validators),
		"dump_consensus_state": newMethod(s.DumpConsensusState),
		"consensus_state":      newMethod(s.GetConsensusState),
		"consensus_params":     newMethod(s.ConsensusParams),
		"unconfirmed_txs":      newMethod(s.UnconfirmedTxs),
		"num_unconfirmed_txs":  newMethod(s.NumUnconfirmedTxs),
		"broadcast_tx_commit":  newMethod(s.BroadcastTxCommit),
		"broadcast_tx_sync":    newMethod(s.BroadcastTxSync),
		"broadcast_tx_async":   newMethod(s.BroadcastTxAsync),
		"abci_query":           newMethod(s.ABCIQuery),
		"abci_info":            newMethod(s.ABCIInfo),
		"broadcast_evidence":   newMethod(s.BroadcastEvidence),
	}
	return &s
}

func (s *service) Subscribe(req *http.Request, args *SubscribeArgs) (*ctypes.ResultSubscribe, error) {
	return nil, errors.New("not implemented")
}

func (s *service) Unsubscribe(req *http.Request, args *UnsubscribeArgs) (*EmptyResult, error) {
	return nil, errors.New("not implemented")
}

func (s *service) UnsubscribeAll(req *http.Request, args *UnsubscribeAllArgs) (*EmptyResult, error) {
	return nil, errors.New("not implemented")
}

// info API
func (s *service) Health(req *http.Request, args *HealthArgs) (*ctypes.ResultHealth, error) {
	return s.client.Health(req.Context())
}

func (s *service) Status(req *http.Request, args *StatusArgs) (*ctypes.ResultStatus, error) {
	return s.client.Status(req.Context())
}

func (s *service) NetInfo(req *http.Request, args *NetInfoArgs) (*ctypes.ResultNetInfo, error) {
	return s.client.NetInfo(req.Context())
}

func (s *service) BlockchainInfo(req *http.Request, args *BlockchainInfoArgs) (*ctypes.ResultBlockchainInfo, error) {
	return s.client.BlockchainInfo(req.Context(), int64(args.MinHeight), int64(args.MaxHeight))
}

func (s *service) Genesis(req *http.Request, args *GenesisArgs) (*ctypes.ResultGenesis, error) {
	return s.client.Genesis(req.Context())
}

func (s *service) GenesisChunked(req *http.Request, args *GenesisChunkedArgs) (*ctypes.ResultGenesisChunk, error) {
	return s.client.GenesisChunked(req.Context(), uint(args.Id))
}

func (s *service) Block(req *http.Request, args *BlockArgs) (*ctypes.ResultBlock, error) {
	return s.client.Block(req.Context(), (*int64)(&args.Height))
}

func (s *service) BlockByHash(req *http.Request, args *BlockByHashArgs) (*ctypes.ResultBlock, error) {
	return s.client.BlockByHash(req.Context(), args.Hash)
}

func (s *service) BlockResults(req *http.Request, args *BlockResultsArgs) (*ctypes.ResultBlockResults, error) {
	return s.client.BlockResults(req.Context(), (*int64)(&args.Height))
}

func (s *service) Commit(req *http.Request, args *CommitArgs) (*ctypes.ResultCommit, error) {
	return s.client.Commit(req.Context(), (*int64)(&args.Height))
}

func (s *service) CheckTx(req *http.Request, args *CheckTxArgs) (*ctypes.ResultCheckTx, error) {
	return s.client.CheckTx(req.Context(), args.Tx)
}

func (s *service) Tx(req *http.Request, args *TxArgs) (*ctypes.ResultTx, error) {
	return s.client.Tx(req.Context(), args.Hash, args.Prove)
}

func (s *service) TxSearch(req *http.Request, args *TxSearchArgs) (*ctypes.ResultTxSearch, error) {
	return s.client.TxSearch(req.Context(), args.Query, args.Prove, (*int)(&args.Page), (*int)(&args.PerPage), args.OrderBy)
}

func (s *service) BlockSearch(req *http.Request, args *BlockSearchArgs) (*ctypes.ResultBlockSearch, error) {
	return s.client.BlockSearch(req.Context(), args.Query, (*int)(&args.Page), (*int)(&args.PerPage), args.OrderBy)
}

func (s *service) Validators(req *http.Request, args *ValidatorsArgs) (*ctypes.ResultValidators, error) {
	return s.client.Validators(req.Context(), (*int64)(&args.Height), (*int)(&args.Page), (*int)(&args.PerPage))
}

func (s *service) DumpConsensusState(req *http.Request, args *DumpConsensusStateArgs) (*ctypes.ResultDumpConsensusState, error) {
	return s.client.DumpConsensusState(req.Context())
}

func (s *service) GetConsensusState(req *http.Request, args *GetConsensusStateArgs) (*ctypes.ResultConsensusState, error) {
	return s.client.ConsensusState(req.Context())
}

func (s *service) ConsensusParams(req *http.Request, args *ConsensusParamsArgs) (*ctypes.ResultConsensusParams, error) {
	return s.client.ConsensusParams(req.Context(), (*int64)(&args.Height))
}

func (s *service) UnconfirmedTxs(req *http.Request, args *UnconfirmedTxsArgs) (*ctypes.ResultUnconfirmedTxs, error) {
	return s.client.UnconfirmedTxs(req.Context(), (*int)(&args.Limit))
}

func (s *service) NumUnconfirmedTxs(req *http.Request, args *NumUnconfirmedTxsArgs) (*ctypes.ResultUnconfirmedTxs, error) {
	return s.client.NumUnconfirmedTxs(req.Context())
}

// tx broadcast API
func (s *service) BroadcastTxCommit(req *http.Request, args *BroadcastTxCommitArgs) (*ctypes.ResultBroadcastTxCommit, error) {
	return s.client.BroadcastTxCommit(req.Context(), args.Tx)
}

func (s *service) BroadcastTxSync(req *http.Request, args *BroadcastTxSyncArgs) (*ctypes.ResultBroadcastTx, error) {
	return s.client.BroadcastTxSync(req.Context(), args.Tx)
}

func (s *service) BroadcastTxAsync(req *http.Request, args *BroadcastTxAsyncArgs) (*ctypes.ResultBroadcastTx, error) {
	return s.client.BroadcastTxAsync(req.Context(), args.Tx)
}

// abci API
func (s *service) ABCIQuery(req *http.Request, args *ABCIQueryArgs) (*ctypes.ResultABCIQuery, error) {
	return s.client.ABCIQueryWithOptions(req.Context(), args.Path, args.Data, rpcclient.ABCIQueryOptions{
		Height: int64(args.Height),
		Prove:  args.Prove,
	})
}

func (s *service) ABCIInfo(req *http.Request, args *ABCIInfoArgs) (*ctypes.ResultABCIInfo, error) {
	return s.client.ABCIInfo(req.Context())
}

// evidence API
func (s *service) BroadcastEvidence(req *http.Request, args *BroadcastEvidenceArgs) (*ctypes.ResultBroadcastEvidence, error) {
	return s.client.BroadcastEvidence(req.Context(), args.Evidence)
}
