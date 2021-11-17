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
	Id uint
}
type BlockArgs struct {
	Height int64
}
type BlockByHashArgs struct {
	Hash []byte
}
type BlockResultsArgs struct {
	Height int64
}
type CommitArgs struct {
	Height int64
}
type CheckTxArgs struct {
	Tx types.Tx
}
type TxArgs struct {
	Hash  []byte
	Prove bool
}
type TxSearchArgs struct {
	Query   string
	Proove  bool
	Page    int
	PerPage int
	OrderBy string
}
type BlockSearchArgs struct {
	Query   string
	Page    int
	PerPage int
	OrderBy string
}
type ValidatorsArgs struct {
	Height  int64
	Page    int
	PerPage int
}
type DumpConsensusStateArgs struct {
}
type GetConsensusStateArgs struct {
}
type ConsensusParamsArgs struct {
	Height int64
}
type UnconfirmedTxsArgs struct {
	Limit int
}
type NumUnconfirmedTxsArgs struct {
}

// tx broadcast API
type BroadcastTxCommitArgs struct {
	Tx types.Tx
}
type BroadcastTxSyncArgs struct {
	Tx types.Tx
}
type BroadcastTxAsyncArgs struct {
	Tx types.Tx
}

// abci API
type ABCIQueryArgs struct {
	Path string
	Data bytes.HexBytes
}
type ABCIInfoArgs struct {
}

// evidence API
type BroadcastEvidenceArgs struct {
	Evidence types.Evidence
}

type EmptyResult struct{}
