package store

import (
	"strconv"

	"github.com/rollkit/rollkit/types"
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
