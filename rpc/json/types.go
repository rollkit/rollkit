package json

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"

	"github.com/cometbft/cometbft/libs/bytes"
	"github.com/cometbft/cometbft/types"
	"github.com/gorilla/rpc/v2/json2"
)

type subscribeArgs struct {
	Query *string `json:"query"`
}

type unsubscribeArgs struct {
	Query *string `json:"query"`
}

type unsubscribeAllArgs struct{}

// info API
type healthArgs struct {
}
type statusArgs struct {
}
type netInfoArgs struct {
}
type blockchainInfoArgs struct {
	MinHeight *StrInt64 `json:"minHeight"`
	MaxHeight *StrInt64 `json:"maxHeight"`
}

type genesisArgs struct{}

type genesisChunkedArgs struct {
	ID StrInt `json:"chunk"`
}

type blockArgs struct {
	Height *BlockNumber `json:"height"`
}

type blockByHashArgs struct {
	Hash []byte `json:"hash"`
}

type blockResultsArgs struct {
	Height *StrInt64 `json:"height"`
}

type commitArgs struct {
	Height *StrInt64 `json:"height"`
}

type headerArgs struct {
	Height *StrInt64 `json:"height"`
}

type headerByHashArgs struct {
	Hash []byte `json:"hash"`
}

type checkTxArgs struct {
	Tx types.Tx `json:"tx"`
}

type txArgs struct {
	Hash  []byte `json:"hash"`
	Prove bool   `json:"prove"`
}

type txSearchArgs struct {
	Query   string  `json:"query"`
	Prove   bool    `json:"prove"`
	Page    *StrInt `json:"page"`
	PerPage *StrInt `json:"per_page"`
	OrderBy *string `json:"order_by"`
}

type blockSearchArgs struct {
	Query   string  `json:"query"`
	Page    *StrInt `json:"page"`
	PerPage *StrInt `json:"per_page"`
	OrderBy *string `json:"order_by"`
}

type validatorsArgs struct {
	Height  *StrInt64 `json:"height"`
	Page    *StrInt   `json:"page"`
	PerPage *StrInt   `json:"per_page"`
}

type dumpConsensusStateArgs struct{}

type getConsensusStateArgs struct{}

type consensusParamsArgs struct {
	Height *StrInt64 `json:"height"`
}

type unconfirmedTxsArgs struct {
	Limit *StrInt `json:"limit"`
}

type numUnconfirmedTxsArgs struct{}

// tx broadcast API
type broadcastTxCommitArgs struct {
	Tx types.Tx `json:"tx"`
}
type broadcastTxSyncArgs struct {
	Tx types.Tx `json:"tx"`
}
type broadcastTxAsyncArgs struct {
	Tx types.Tx `json:"tx"`
}

// abci API

// ABCIQueryArgs defines args for ABCI Query method.
type ABCIQueryArgs struct {
	Path   string         `json:"path"`
	Data   bytes.HexBytes `json:"data"`
	Height *StrInt64      `json:"height"`
	Prove  *bool          `json:"prove"`
}

// ABCIInfoArgs defines args for ABCI Info method.
type ABCIInfoArgs struct {
}

// evidence API

type broadcastEvidenceArgs struct {
	Evidence types.Evidence `json:"evidence"`
}

type emptyResult struct{}

// JSON-deserialization specific types

// StrInt is an proper int or quoted "int"
type StrInt int

// StrInt64 is an proper int64 or quoted "int64"
type StrInt64 int64

// UnmarshalJSON parses JSON (int or int quoted as string) into StrInt64
func (s *StrInt64) UnmarshalJSON(b []byte) error {
	return unmarshalStrInt64(b, s)
}

// UnmarshalJSON parses JSON (int or int quoted as string) into StrInt
func (s *StrInt) UnmarshalJSON(b []byte) error {
	var val StrInt64
	err := unmarshalStrInt64(b, &val)
	*s = StrInt(val)
	return err
}

// BlockNumber is a StrInt64 with helper tags for block heights
type BlockNumber StrInt64

// BlockNumber tags:
// - "earliest" = literal 1
// - "included" = seen on DA
const (
	IncludedBlockNumber = BlockNumber(-1)
	EarliestBlockNumber = BlockNumber(1)
)

func unmarshalBlockNumber(b []byte, bn *BlockNumber) error {
	input := strings.TrimSpace(string(b))
	if len(input) >= 2 && input[0] == '"' && input[len(input)-1] == '"' {
		input = input[1 : len(input)-1]
	}

	switch input {
	case "earliest":
		*bn = EarliestBlockNumber
	case "included":
		*bn = IncludedBlockNumber
	default:
		// Try to parse as int64
		if i, err := strconv.ParseInt(input, 10, 64); err == nil {
			*bn = BlockNumber(i)
		} else {
			// If parsing as int64 fails, try to unmarshal as JSON number
			var f float64
			if err := json.Unmarshal(b, &f); err == nil {
				*bn = BlockNumber(f)
			} else {
				return &json.UnsupportedValueError{
					Value: reflect.ValueOf(input),
					Str:   string(b),
				}
			}
		}
	}

	return nil
}

// UnmarshalJSON parses JSON (int or block tag quoted as string) into BlockNumber
func (bn *BlockNumber) UnmarshalJSON(b []byte) error {
	return unmarshalBlockNumber(b, bn)
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
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *json2.Error    `json:"error,omitempty"`
	ID      json.RawMessage `json:"id"`
}
