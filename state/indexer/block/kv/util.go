package kv

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/cometbft/cometbft/libs/pubsub/query"
	"github.com/cometbft/cometbft/types"
	"github.com/rollkit/rollkit/store"
)

func intInSlice(a int, list []int) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}

	return false
}

func int64FromBytes(bz []byte) int64 {
	v, _ := binary.Varint(bz)
	return v
}

func int64ToBytes(i int64) []byte {
	buf := make([]byte, binary.MaxVarintLen64)
	n := binary.PutVarint(buf, i)
	return buf[:n]
}

func heightKey(height int64) string {
	return store.GenerateKey([]interface{}{types.BlockHeightKey, height})
}

func eventKey(compositeKey, typ, eventValue string, height int64) string {
	return store.GenerateKey([]interface{}{compositeKey, eventValue, height, typ})
}

func parseValueFromPrimaryKey(key string) (int64, error) {
	parts := strings.SplitN(key, "/", 3)
	height, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse event key: %w", err)
	}
	return height, nil
}

func parseValueFromEventKey(key string) string {
	parts := strings.SplitN(key, "/", 5)
	return parts[2]
}

func lookForHeight(conditions []query.Condition) (int64, bool) {
	for _, c := range conditions {
		if c.CompositeKey == types.BlockHeightKey && c.Op == query.OpEqual {
			return c.Operand.(*big.Int).Int64(), true
		}
	}

	return 0, false
}
