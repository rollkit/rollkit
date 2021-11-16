package json

import (
	"errors"
	"net/http"

	ctypes "github.com/tendermint/tendermint/rpc/core/types"

	"github.com/celestiaorg/optimint/rpc/client"
	gorillarpc "github.com/gorilla/rpc/v2"
)

const serviceName = "optimint"

func getServiceName(method string) string {
	return serviceName + "." + method
}

func GetHttpHandler(l *client.Client) (http.Handler, error) {
	s := gorillarpc.NewServer()
	aliases := map[string]string{
		"subscribe":            getServiceName("Subscribe"),
		"unsubscribe":          getServiceName("Unsubscribe"),
		"unsubscribe_all":      getServiceName("UnsubscribeAll"),
		"health":               getServiceName("Health"),
		"status":               getServiceName("Status"),
		"net_info":             getServiceName("NetInfo"),
		"blockchain":           getServiceName("BlockchainInfo"),
		"genesis":              getServiceName("Genesis"),
		"genesis_chunked":      getServiceName("GenesisChunked"),
		"block":                getServiceName("Block"),
		"block_by_hash":        getServiceName("BlockByHash"),
		"block_results":        getServiceName("BlockResults"),
		"commit":               getServiceName("Commit"),
		"check_tx":             getServiceName("CheckTx"),
		"tx":                   getServiceName("Tx"),
		"tx_search":            getServiceName("TxSearch"),
		"block_search":         getServiceName("BlockSearch"),
		"validators":           getServiceName("Validators"),
		"dump_consensus_state": getServiceName("DumpConsensusState"),
		"consensus_state":      getServiceName("GetConsensusState"),
		"consensus_params":     getServiceName("ConsensusParams"),
		"unconfirmed_txs":      getServiceName("UnconfirmedTxs"),
		"num_unconfirmed_txs":  getServiceName("NumUnconfirmedTxs"),
		"broadcast_tx_commit":  getServiceName("BroadcastTxCommit"),
		"broadcast_tx_sync":    getServiceName("BroadcastTxSync"),
		"broadcast_tx_async":   getServiceName("BroadcastTxAsync"),
		"abci_query":           getServiceName("ABCIQuery"),
		"abci_info":            getServiceName("ABCIInfo"),

		"broadcast_evidence": getServiceName("BroadcastEvidence"),
	}
	s.RegisterCodec(NewMapperCodec(aliases), "application/json")
	err := s.RegisterService(&service{l: l}, serviceName)
	return s, err
}

type service struct {
	l *client.Client
}

func (s *service) Subscribe(req *http.Request, args *SubscribeArgs, resp *ctypes.ResultSubscribe) error {
	return errors.New("not implemented")
}

func (s *service) Unsubscribe(req *http.Request, args *UnsubscribeArgs, resp *EmptyResult) error {
	return errors.New("not implemented")
}

func (s *service) UnsubscribeAll(req *http.Request, args *UnsubscribeAllArgs, resp *EmptyResult) error {
	return errors.New("not implemented")
}

// info API
func (s *service) Health(req *http.Request, args *HealthArgs, resp *ctypes.ResultHealth) (err error) {
	resp, err = s.l.Health(req.Context())
	return
}

func (s *service) Status(req *http.Request, args *StatusArgs, resp *ctypes.ResultStatus) (err error) {
	resp, err = s.l.Status(req.Context())
	return
}

func (s *service) NetInfo(req *http.Request, args *NetInfoArgs, resp *ctypes.ResultNetInfo) (err error) {
	resp, err = s.l.NetInfo(req.Context())
	return
}

func (s *service) BlockchainInfo(req *http.Request, args *BlockchainInfoArgs, resp *ctypes.ResultBlockchainInfo) (err error) {
	resp, err = s.l.BlockchainInfo(req.Context(), args.MinHeight, args.MaxHeight)
	return
}

func (s *service) Genesis(req *http.Request, args *GenesisArgs, resp *ctypes.ResultGenesis) (err error) {
	resp, err = s.l.Genesis(req.Context())
	return
}

func (s *service) GenesisChunked(req *http.Request, args *GenesisChunkedArgs, resp *ctypes.ResultGenesisChunk) (err error) {
	resp, err = s.l.GenesisChunked(req.Context(), args.Id)
	return
}

func (s *service) Block(req *http.Request, args *BlockArgs, resp *ctypes.ResultBlock) (err error) {
	resp, err = s.l.Block(req.Context(), &args.Height)
	return
}

func (s *service) BlockByHash(req *http.Request, args *BlockByHashArgs, resp *ctypes.ResultBlock) (err error) {
	resp, err = s.l.BlockByHash(req.Context(), args.Hash)
	return
}

func (s *service) BlockResults(req *http.Request, args *BlockResultsArgs, resp *ctypes.ResultBlockResults) (err error) {
	resp, err = s.l.BlockResults(req.Context(), &args.Height)
	return
}

func (s *service) Commit(req *http.Request, args *CommitArgs, resp *ctypes.ResultCommit) (err error) {
	resp, err = s.l.Commit(req.Context(), &args.Height)
	return
}

func (s *service) CheckTx(req *http.Request, args *CheckTxArgs, resp *ctypes.ResultCheckTx) (err error) {
	resp, err = s.l.CheckTx(req.Context(), args.Tx)
	return
}

func (s *service) Tx(req *http.Request, args *TxArgs, resp *ctypes.ResultTx) (err error) {
	resp, err = s.l.Tx(req.Context(), args.Hash, args.Prove)
	return
}

func (s *service) TxSearch(req *http.Request, args *TxSearchArgs, resp *ctypes.ResultTxSearch) (err error) {
	resp, err = s.l.TxSearch(req.Context(), args.Query, args.Proove, &args.Page, &args.PerPage, args.OrderBy)
	return
}

func (s *service) BlockSearch(req *http.Request, args *BlockSearchArgs, resp *ctypes.ResultBlockSearch) (err error) {
	resp, err = s.l.BlockSearch(req.Context(), args.Query, &args.Page, &args.PerPage, args.OrderBy)
	return
}

func (s *service) Validators(req *http.Request, args *ValidatorsArgs, resp *ctypes.ResultValidators) (err error) {
	resp, err = s.l.Validators(req.Context(), &args.Height, &args.Page, &args.PerPage)
	return
}

func (s *service) DumpConsensusState(req *http.Request, args *DumpConsensusStateArgs, resp *ctypes.ResultDumpConsensusState) (err error) {
	resp, err = s.l.DumpConsensusState(req.Context())
	return
}

func (s *service) GetConsensusState(req *http.Request, args *GetConsensusStateArgs, resp *ctypes.ResultConsensusState) (err error) {
	resp, err = s.l.ConsensusState(req.Context())
	return
}

func (s *service) ConsensusParams(req *http.Request, args *ConsensusParamsArgs, resp *ctypes.ResultConsensusParams) (err error) {
	resp, err = s.l.ConsensusParams(req.Context(), &args.Height)
	return
}

func (s *service) UnconfirmedTxs(req *http.Request, args *UnconfirmedTxsArgs, resp *ctypes.ResultUnconfirmedTxs) (err error) {
	resp, err = s.l.UnconfirmedTxs(req.Context(), &args.Limit)
	return
}

func (s *service) NumUnconfirmedTxs(req *http.Request, args *NumUnconfirmedTxsArgs, resp *ctypes.ResultUnconfirmedTxs) (err error) {
	resp, err = s.l.NumUnconfirmedTxs(req.Context())
	return
}

// tx broadcast API
func (s *service) BroadcastTxCommit(req *http.Request, args *BroadcastTxCommitArgs, resp *ctypes.ResultBroadcastTxCommit) (err error) {
	resp, err = s.l.BroadcastTxCommit(req.Context(), args.Tx)
	return
}

func (s *service) BroadcastTxSync(req *http.Request, args *BroadcastTxSyncArgs, resp *ctypes.ResultBroadcastTx) (err error) {
	resp, err = s.l.BroadcastTxSync(req.Context(), args.Tx)
	return
}

func (s *service) BroadcastTxAsync(req *http.Request, args *BroadcastTxAsyncArgs, resp *ctypes.ResultBroadcastTx) (err error) {
	resp, err = s.l.BroadcastTxAsync(req.Context(), args.Tx)
	return
}

// abci API
func (s *service) ABCIQuery(req *http.Request, args *ABCIQueryArgs, resp *ctypes.ResultABCIQuery) (err error) {
	resp, err = s.l.ABCIQuery(req.Context(), args.Path, args.Data)
	return
}

func (s *service) ABCIInfo(req *http.Request, args *ABCIInfoArgs, resp *ctypes.ResultABCIInfo) (err error) {
	resp, err = s.l.ABCIInfo(req.Context())
	return
}

// evidence API
func (s *service) BroadcastEvidence(req *http.Request, args *BroadcastEvidenceArgs, resp *ctypes.ResultBroadcastEvidence) (err error) {
	resp, err = s.l.BroadcastEvidence(req.Context(), args.Evidence)
	return
}
