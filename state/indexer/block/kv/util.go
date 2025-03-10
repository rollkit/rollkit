package kv

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"strings"

	"github.com/cometbft/cometbft/libs/pubsub/query/syntax"
	"github.com/cometbft/cometbft/types"

	"github.com/rollkit/rollkit/state"
	"github.com/rollkit/rollkit/state/indexer"
	"github.com/rollkit/rollkit/store"
)

type HeightInfo struct {
	heightRange     indexer.QueryRange
	height          int64
	heightEqIdx     int
	onlyHeightRange bool
	onlyHeightEq    bool
}

func intInSlice(a int, list []int) bool {
	return slices.Contains(list, a)
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
	return store.GenerateKey([]string{types.BlockHeightKey, strconv.FormatInt(height, 10)})
}

func eventKey(compositeKey, typ, eventValue string, height int64) string {
	return store.GenerateKey([]string{compositeKey, eventValue, strconv.FormatInt(height, 10), typ})
}

func parseValueFromPrimaryKey(key string) string {
	parts := strings.SplitN(key, "/", 3)
	return parts[2]
	// height, err := strconv.ParseFloat(parts[2], 64)
	// if err != nil {
	// 	return 0, fmt.Errorf("failed to parse event key: %w", err)
	// }
	// return height, nil
}

func parseHeightFromEventKey(key string) (int64, error) {
	parts := strings.SplitN(key, "/", 5)
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

// Remove all occurrences of height equality queries except one. While we are traversing the conditions, check whether the only condition in
// addition to match events is the height equality or height range query. At the same time, if we do have a height range condition
// ignore the height equality condition. If a height equality exists, place the condition index in the query and the desired height
// into the heightInfo struct
func dedupHeight(conditions []syntax.Condition) (dedupConditions []syntax.Condition, heightInfo HeightInfo, found bool) {
	heightInfo.heightEqIdx = -1
	heightRangeExists := false
	var heightCondition []syntax.Condition
	heightInfo.onlyHeightEq = true
	heightInfo.onlyHeightRange = true
	for _, c := range conditions {
		if c.Tag == types.BlockHeightKey {
			if c.Op == syntax.TEq {
				if found || heightRangeExists {
					continue
				}
				hFloat := c.Arg.Number()
				if hFloat != nil {
					h, _ := hFloat.Int64()
					heightInfo.height = h
					heightCondition = append(heightCondition, c)
					found = true
				}
			} else {
				heightInfo.onlyHeightEq = false
				heightRangeExists = true
				dedupConditions = append(dedupConditions, c)
			}
		} else {
			heightInfo.onlyHeightRange = false
			heightInfo.onlyHeightEq = false
			dedupConditions = append(dedupConditions, c)
		}
	}
	if !heightRangeExists && len(heightCondition) != 0 {
		heightInfo.heightEqIdx = len(dedupConditions)
		heightInfo.onlyHeightRange = false
		dedupConditions = append(dedupConditions, heightCondition...)
	} else {
		// If we found a range make sure we set the hegiht idx to -1 as the height equality
		// will be removed
		heightInfo.heightEqIdx = -1
		heightInfo.height = 0
		heightInfo.onlyHeightEq = false
		found = false
	}
	return dedupConditions, heightInfo, found
}

func checkHeightConditions(heightInfo HeightInfo, keyHeight int64) (bool, error) {
	if heightInfo.heightRange.Key != "" {
		withinBounds, err := state.CheckBounds(heightInfo.heightRange, big.NewInt(keyHeight))
		if err != nil || !withinBounds {
			return false, err
		}
	} else {
		if heightInfo.height != 0 && keyHeight != heightInfo.height {
			return false, nil
		}
	}
	return true, nil
}
