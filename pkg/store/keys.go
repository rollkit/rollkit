package store

import (
	"strconv"

	"github.com/rollkit/rollkit/types"
)

const (
	// LastSubmittedHeightKey is the key used for persisting the last submitted height in store.
	LastSubmittedHeightKey = "last submitted"
	// DAIncludedHeightKey is the key used for persisting the da included height in store.
	DAIncludedHeightKey = "d"

	// LastBatchDataKey is the key used for persisting the last batch data in store.
	LastBatchDataKey = "l"
)

func getHeaderKey(height uint64) string {
	return GenerateKey([]string{headerPrefix, strconv.FormatUint(height, 10)})
}

func getDataKey(height uint64) string {
	return GenerateKey([]string{dataPrefix, strconv.FormatUint(height, 10)})
}

func getSignatureKey(height uint64) string {
	return GenerateKey([]string{signaturePrefix, strconv.FormatUint(height, 10)})
}

func getStateKey() string {
	return statePrefix
}

func getMetaKey(key string) string {
	return GenerateKey([]string{metaPrefix, key})
}

func getIndexKey(hash types.Hash) string {
	return GenerateKey([]string{indexPrefix, hash.String()})
}

func getHeightKey() string {
	return GenerateKey([]string{heightPrefix})
}
