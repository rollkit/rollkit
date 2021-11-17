package json

import (
	"github.com/tendermint/tendermint/libs/bytes"
	"github.com/tendermint/tendermint/types"
)

type SubscribeArgs struct {
}
type UnsubscribeArgs struct {
}
type UnsubscribeAllArgs struct {
}

// info API
type HealthArgs struct {
}
type StatusArgs struct {
}
type NetInfoArgs struct {
}
type BlockchainInfoArgs struct {
	MinHeight int64
	MaxHeight int64
}
type GenesisArgs struct {
}
type GenesisChunkedArgs struct {
	Id uint `json:"chunk"`
}
type BlockArgs struct {
	Height int64 `json:"height"`
}
type BlockByHashArgs struct {
	Hash []byte `json:"hash"`
}
type BlockResultsArgs struct {
	Height int64 `json:"height"`
}
type CommitArgs struct {
	Height int64 `json:"height"`
}
type CheckTxArgs struct {
	Tx types.Tx `json:"tx"`
}
type TxArgs struct {
	Hash  []byte `json:"hash"`
	Prove bool   `json:"prove"`
}
type TxSearchArgs struct {
	Query   string `json:"query"`
	Prove   bool   `json:"prove"`
	Page    int    `json:"page"`
	PerPage int    `json:"per_page"`
	OrderBy string `json:"order_by"`
}
type BlockSearchArgs struct {
	Query   string `json:"query"`
	Page    int    `json:"page"`
	PerPage int    `json:"per_page"`
	OrderBy string `json:"order_by"`
}
type ValidatorsArgs struct {
	Height  int64 `json:"height"`
	Page    int   `json:"page"`
	PerPage int   `json:"per_page"`
}
type DumpConsensusStateArgs struct {
}
type GetConsensusStateArgs struct {
}
type ConsensusParamsArgs struct {
	Height int64 `json:"height"`
}
type UnconfirmedTxsArgs struct {
	Limit int `json:"limit"`
}
type NumUnconfirmedTxsArgs struct {
}

// tx broadcast API
type BroadcastTxCommitArgs struct {
	Tx types.Tx `json:"tx"`
}
type BroadcastTxSyncArgs struct {
	Tx types.Tx `json:"tx"`
}
type BroadcastTxAsyncArgs struct {
	Tx types.Tx `json:"tx"`
}

// abci API
type ABCIQueryArgs struct {
	Path   string         `json:"path"`
	Data   bytes.HexBytes `json:"data"`
	Height int64          `json:"height"`
	Prove  bool           `json:"prove"`
}
type ABCIInfoArgs struct {
}

// evidence API
type BroadcastEvidenceArgs struct {
	Evidence types.Evidence `json:"evidence"`
}

type EmptyResult struct{}
