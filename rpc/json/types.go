package json

import (
	"encoding/json"
	"reflect"
	"strconv"

	"github.com/gorilla/rpc/v2/json2"
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
	MinHeight StrInt64
	MaxHeight StrInt64
}
type GenesisArgs struct {
}
type GenesisChunkedArgs struct {
	Id StrInt `json:"chunk"`
}
type BlockArgs struct {
	Height StrInt64 `json:"height"`
}
type BlockByHashArgs struct {
	Hash []byte `json:"hash"`
}
type BlockResultsArgs struct {
	Height StrInt64 `json:"height"`
}
type CommitArgs struct {
	Height StrInt64 `json:"height"`
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
	Page    StrInt `json:"page"`
	PerPage StrInt `json:"per_page"`
	OrderBy string `json:"order_by"`
}
type BlockSearchArgs struct {
	Query   string `json:"query"`
	Page    StrInt `json:"page"`
	PerPage StrInt `json:"per_page"`
	OrderBy string `json:"order_by"`
}
type ValidatorsArgs struct {
	Height  StrInt64 `json:"height"`
	Page    StrInt   `json:"page"`
	PerPage StrInt   `json:"per_page"`
}
type DumpConsensusStateArgs struct {
}
type GetConsensusStateArgs struct {
}
type ConsensusParamsArgs struct {
	Height StrInt64 `json:"height"`
}
type UnconfirmedTxsArgs struct {
	Limit StrInt `json:"limit"`
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
	Height StrInt64       `json:"height"`
	Prove  bool           `json:"prove"`
}
type ABCIInfoArgs struct {
}

// evidence API
type BroadcastEvidenceArgs struct {
	Evidence types.Evidence `json:"evidence"`
}

type EmptyResult struct{}

// JSON-deserialization specific types

// StrInt is an proper int or quoted "int"
type StrInt int

// StrInt64 is an proper int64 or quoted "int64"
type StrInt64 int64

func (s *StrInt64) UnmarshalJSON(b []byte) error {
	return unmarshalStrInt64(b, s)
}

func (s *StrInt) UnmarshalJSON(b []byte) error {
	var val StrInt64
	err := unmarshalStrInt64(b, &val)
	*s = StrInt(val)
	return err
}

func unmarshalStrInt64(b []byte, s *StrInt64) error {
	var i interface{}
	err := json.Unmarshal(b, &i)
	if err != nil {
		return err
	}

	switch v := i.(type) {
	case int:
		*s = StrInt64(v)
	case int64:
		*s = StrInt64(v)
	case string:
		iv, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		*s = StrInt64(iv)
	default:
		return &json.UnsupportedValueError{
			Value: reflect.ValueOf(i),
			Str:   string(b),
		}
	}
	return nil
}

type response struct {
	Version string          `json:"jsonrpc"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *json2.Error    `json:"error,omitempty"`
	Id      json.RawMessage `json:"id"`
}
