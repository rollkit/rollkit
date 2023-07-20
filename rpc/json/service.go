package json

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"time"

	rpcclient "github.com/cometbft/cometbft/rpc/client"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/gorilla/rpc/v2/json2"

	"github.com/rollkit/rollkit/log"
)

// GetHTTPHandler returns handler configured to serve Tendermint-compatible RPC.
func GetHTTPHandler(l rpcclient.Client, logger log.Logger) (http.Handler, error) {
	return newHandler(newService(l, logger), json2.NewCodec(), logger), nil
}

type method struct {
	m          reflect.Value
	argsType   reflect.Type
	returnType reflect.Type
	ws         bool
}

func newMethod(m interface{}) *method {
	mType := reflect.TypeOf(m)

	return &method{
		m:          reflect.ValueOf(m),
		argsType:   mType.In(1).Elem(),
		returnType: mType.Out(0).Elem(),
		ws:         mType.NumIn() == 3,
	}
}

type service struct {
	client  rpcclient.Client
	methods map[string]*method
	logger  log.Logger
}

func newService(c rpcclient.Client, l log.Logger) *service {
	s := service{
		client: c,
		logger: l,
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

func (s *service) Subscribe(req *http.Request, args *subscribeArgs, wsConn *wsConn) (*ctypes.ResultSubscribe, error) {
	// TODO(tzdybal): pass config and check subscriptions limits
	// TODO(tzdybal): extract consts or configs
	const SubscribeTimeout = 5 * time.Second
	const subBufferSize = 100

	addr := req.RemoteAddr
	query := args.Query

	ctx, cancel := context.WithTimeout(req.Context(), SubscribeTimeout)
	defer cancel()

	sub, err := s.client.Subscribe(ctx, addr, query, subBufferSize)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe: %w", err)
	}

	go func() {
		for msg := range sub {
			data, err := json.Marshal(msg.Data)
			if err != nil {
				s.logger.Error("failed to marshal response data", "error", err)
				continue
			}
			if wsConn != nil {
				wsConn.queue <- data
			}
		}
	}()

	return &ctypes.ResultSubscribe{}, nil
}

func (s *service) Unsubscribe(req *http.Request, args *unsubscribeArgs) (*emptyResult, error) {
	s.logger.Debug("unsubscribe from query", "remote", req.RemoteAddr, "query", args.Query)
	err := s.client.Unsubscribe(context.Background(), req.RemoteAddr, args.Query)
	if err != nil {
		return nil, fmt.Errorf("failed to unsubscribe: %w", err)
	}
	return &emptyResult{}, nil
}

func (s *service) UnsubscribeAll(req *http.Request, args *unsubscribeAllArgs) (*emptyResult, error) {
	s.logger.Debug("unsubscribe from all queries", "remote", req.RemoteAddr)
	err := s.client.UnsubscribeAll(context.Background(), req.RemoteAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to unsubscribe all: %w", err)
	}
	return &emptyResult{}, nil
}

// info API
func (s *service) Health(req *http.Request, args *healthArgs) (*ctypes.ResultHealth, error) {
	return s.client.Health(req.Context())
}

func (s *service) Status(req *http.Request, args *statusArgs) (*ctypes.ResultStatus, error) {
	return s.client.Status(req.Context())
}

func (s *service) NetInfo(req *http.Request, args *netInfoArgs) (*ctypes.ResultNetInfo, error) {
	return s.client.NetInfo(req.Context())
}

func (s *service) BlockchainInfo(req *http.Request, args *blockchainInfoArgs) (*ctypes.ResultBlockchainInfo, error) {
	return s.client.BlockchainInfo(req.Context(), int64(args.MinHeight), int64(args.MaxHeight))
}

func (s *service) Genesis(req *http.Request, args *genesisArgs) (*ctypes.ResultGenesis, error) {
	return s.client.Genesis(req.Context())
}

func (s *service) GenesisChunked(req *http.Request, args *genesisChunkedArgs) (*ctypes.ResultGenesisChunk, error) {
	return s.client.GenesisChunked(req.Context(), uint(args.ID))
}

func (s *service) Block(req *http.Request, args *blockArgs) (*ctypes.ResultBlock, error) {
	return s.client.Block(req.Context(), (*int64)(&args.Height))
}

func (s *service) BlockByHash(req *http.Request, args *blockByHashArgs) (*ctypes.ResultBlock, error) {
	return s.client.BlockByHash(req.Context(), args.Hash)
}

func (s *service) BlockResults(req *http.Request, args *blockResultsArgs) (*ctypes.ResultBlockResults, error) {
	return s.client.BlockResults(req.Context(), (*int64)(&args.Height))
}

func (s *service) Commit(req *http.Request, args *commitArgs) (*ctypes.ResultCommit, error) {
	return s.client.Commit(req.Context(), (*int64)(&args.Height))
}

func (s *service) CheckTx(req *http.Request, args *checkTxArgs) (*ctypes.ResultCheckTx, error) {
	return s.client.CheckTx(req.Context(), args.Tx)
}

func (s *service) Tx(req *http.Request, args *txArgs) (*ctypes.ResultTx, error) {
	return s.client.Tx(req.Context(), args.Hash, args.Prove)
}

func (s *service) TxSearch(req *http.Request, args *txSearchArgs) (*ctypes.ResultTxSearch, error) {
	return s.client.TxSearch(req.Context(), args.Query, args.Prove, (*int)(&args.Page), (*int)(&args.PerPage), args.OrderBy)
}

func (s *service) BlockSearch(req *http.Request, args *blockSearchArgs) (*ctypes.ResultBlockSearch, error) {
	return s.client.BlockSearch(req.Context(), args.Query, (*int)(&args.Page), (*int)(&args.PerPage), args.OrderBy)
}

func (s *service) Validators(req *http.Request, args *validatorsArgs) (*ctypes.ResultValidators, error) {
	return s.client.Validators(req.Context(), (*int64)(&args.Height), (*int)(&args.Page), (*int)(&args.PerPage))
}

func (s *service) DumpConsensusState(req *http.Request, args *dumpConsensusStateArgs) (*ctypes.ResultDumpConsensusState, error) {
	return s.client.DumpConsensusState(req.Context())
}

func (s *service) GetConsensusState(req *http.Request, args *getConsensusStateArgs) (*ctypes.ResultConsensusState, error) {
	return s.client.ConsensusState(req.Context())
}

func (s *service) ConsensusParams(req *http.Request, args *consensusParamsArgs) (*ctypes.ResultConsensusParams, error) {
	return s.client.ConsensusParams(req.Context(), (*int64)(&args.Height))
}

func (s *service) UnconfirmedTxs(req *http.Request, args *unconfirmedTxsArgs) (*ctypes.ResultUnconfirmedTxs, error) {
	return s.client.UnconfirmedTxs(req.Context(), (*int)(&args.Limit))
}

func (s *service) NumUnconfirmedTxs(req *http.Request, args *numUnconfirmedTxsArgs) (*ctypes.ResultUnconfirmedTxs, error) {
	return s.client.NumUnconfirmedTxs(req.Context())
}

// tx broadcast API
func (s *service) BroadcastTxCommit(req *http.Request, args *broadcastTxCommitArgs) (*ctypes.ResultBroadcastTxCommit, error) {
	return s.client.BroadcastTxCommit(req.Context(), args.Tx)
}

func (s *service) BroadcastTxSync(req *http.Request, args *broadcastTxSyncArgs) (*ctypes.ResultBroadcastTx, error) {
	return s.client.BroadcastTxSync(req.Context(), args.Tx)
}

func (s *service) BroadcastTxAsync(req *http.Request, args *broadcastTxAsyncArgs) (*ctypes.ResultBroadcastTx, error) {
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
func (s *service) BroadcastEvidence(req *http.Request, args *broadcastEvidenceArgs) (*ctypes.ResultBroadcastEvidence, error) {
	return s.client.BroadcastEvidence(req.Context(), args.Evidence)
}
