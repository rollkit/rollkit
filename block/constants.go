package block

import "time"

const (
	// defaultLazySleepPercent is the percentage of block time to wait to accumulate transactions
	// in lazy mode.
	// A value of 10 for e.g. corresponds to 10% of the block time. Must be between 0 and 100.
	defaultLazySleepPercent = 10

	// defaultDABlockTime is used only if DABlockTime is not configured for manager
	defaultDABlockTime = 15 * time.Second

	// defaultBlockTime is used only if BlockTime is not configured for manager
	defaultBlockTime = 1 * time.Second

	// defaultLazyBlockTime is used only if LazyBlockTime is not configured for manager
	defaultLazyBlockTime = 60 * time.Second

	// defaultMempoolTTL is the number of blocks until transaction is dropped from mempool
	defaultMempoolTTL = 25

	// blockProtocolOverhead is the protocol overhead when marshaling the block to blob
	// see: https://gist.github.com/tuxcanfly/80892dde9cdbe89bfb57a6cb3c27bae2
	blockProtocolOverhead = 1 << 16

	// maxSubmitAttempts defines how many times Rollkit will re-try to publish block to DA layer.
	// This is temporary solution. It will be removed in future versions.
	maxSubmitAttempts = 30

	// Applies to most channels, 100 is a large enough buffer to avoid blocking
	channelLength = 100

	// Applies to the headerInCh, 10000 is a large enough number for headers per DA block.
	headerInChLength = 10000

	// initialBackoff defines initial value for block submission backoff
	initialBackoff = 100 * time.Millisecond

	// DAIncludedHeightKey is the key used for persisting the da included height in store.
	DAIncludedHeightKey = "d"

	// LastBatchDataKey is the key used for persisting the last batch data in store.
	LastBatchDataKey = "l"
)

// dataHashForEmptyTxs to be used while only syncing headers from DA and no p2p to get the Data for no txs scenarios, the syncing can proceed without getting stuck forever.
var dataHashForEmptyTxs = []byte{110, 52, 11, 156, 255, 179, 122, 152, 156, 165, 68, 230, 187, 120, 10, 44, 120, 144, 29, 63, 179, 55, 56, 118, 133, 17, 163, 6, 23, 175, 160, 29}
