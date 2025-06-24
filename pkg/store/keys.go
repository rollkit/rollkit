package store

import (
	"strconv"

	"github.com/rollkit/rollkit/types"
)

const (
	// RollkitHeightToDAHeightKey is the key prefix used for persisting the mapping from a Rollkit height
	// to the DA height where the block's header/data was included.
	// Full keys are like: rhb/<rollkit_height>/h and rhb/<rollkit_height>/d
	RollkitHeightToDAHeightKey = "rhb"

	headerPrefix    = "h"
	dataPrefix      = "d"
	signaturePrefix = "c"
	statePrefix     = "s"
	metaPrefix      = "m"
	indexPrefix     = "i"
	heightPrefix    = "t"
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
