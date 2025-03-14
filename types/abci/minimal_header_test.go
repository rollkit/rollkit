package abci

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/types"
)

func TestMinimalToABCIHeaderTransformation(t *testing.T) {
	// Create a minimal header for testing
	minHeader := &types.MinimalHeader{
		ParentHash:     types.Hash{1, 2, 3},
		BlockHeight:    10,
		BlockTimestamp: uint64(time.Now().UnixNano()),
		BlockChainID:   "test-chain",
		DataCommitment: types.Hash{4, 5, 6},
		StateRoot:      types.Hash{7, 8, 9},
		ExtraData:      []byte("test-extra-data"),
	}

	// Test transformation to ABCI proto header
	pbHeader, err := MinimalToABCIHeaderPB(minHeader)
	require.NoError(t, err)
	assert.Equal(t, int64(minHeader.Height()), pbHeader.Height)
	assert.Equal(t, minHeader.ChainID(), pbHeader.ChainID)

	// Test transformation to ABCI header
	abciHeader, err := MinimalToABCIHeader(minHeader)
	require.NoError(t, err)
	assert.Equal(t, int64(minHeader.Height()), abciHeader.Height)
	assert.Equal(t, minHeader.ChainID(), abciHeader.ChainID)

	// Test transformation to ABCI block
	signedHeader := &types.SignedMinimalHeader{
		Header:    *minHeader,
		Signature: []byte("test-signature"),
	}

	// Create metadata for the Data structure
	metadata := &types.Metadata{
		ChainID:      "test-chain",
		Height:       10,
		Time:         uint64(time.Now().UnixNano()),
		LastDataHash: types.Hash{},
	}

	// Create transactions
	tx1 := types.Tx("tx1")
	tx2 := types.Tx("tx2")
	txs := make(types.Txs, 2)
	txs[0] = tx1
	txs[1] = tx2

	data := &types.Data{
		Metadata: metadata,
		Txs:      txs,
	}

	abciBlock, err := MinimalToABCIBlock(signedHeader, data)
	require.NoError(t, err)
	assert.Equal(t, int64(minHeader.Height()), abciBlock.Header.Height)
	assert.Equal(t, minHeader.ChainID(), abciBlock.Header.ChainID)
	assert.Equal(t, 2, len(abciBlock.Data.Txs))
	assert.Equal(t, []byte("tx1"), []byte(abciBlock.Data.Txs[0]))
	assert.Equal(t, []byte("tx2"), []byte(abciBlock.Data.Txs[1]))
}
