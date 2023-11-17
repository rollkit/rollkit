package types

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/types/pb/rollkit"
)

func TestTxWithISRSerializationRoundtrip(t *testing.T) {
	txs := make(Txs, 1000)
	ISRs := IntermediateStateRoots{
		RawRootsList: make([][]byte, len(txs)+1),
	}
	for i := 0; i < len(txs); i++ {
		txs[i] = GetRandomTx()
		ISRs.RawRootsList[i] = GetRandomBytes(32)
	}
	ISRs.RawRootsList[len(txs)] = GetRandomBytes(32)

	txsWithISRs, err := txs.ToTxsWithISRs(ISRs)
	require.NoError(t, err)
	require.NotEmpty(t, txsWithISRs)

	txShares, err := TxsWithISRsToShares(txsWithISRs)
	require.NoError(t, err)
	require.NotEmpty(t, txShares)

	txBytes, err := SharesToPostableBytes(txShares)
	require.NoError(t, err)
	require.NotEmpty(t, txBytes)

	newTxShares, err := PostableBytesToShares(txBytes)
	require.NoError(t, err)
	require.NotEmpty(t, newTxShares)

	// Note that txShares and newTxShares are not necessarily equal because newTxShares might
	// contain zero padding at the end and thus sequence length can differ
	newTxsWithISRs, err := SharesToTxsWithISRs(newTxShares)
	require.NoError(t, err)
	require.NotEmpty(t, txsWithISRs)

	assert.Equal(t, txsWithISRs, newTxsWithISRs)
}

func TestTxWithISRSerializationOutOfContextRoundtrip(t *testing.T) {
	numTxs := 1000
	txs := make(Txs, numTxs)
	ISRs := IntermediateStateRoots{
		RawRootsList: make([][]byte, len(txs)+1),
	}
	for i := 0; i < len(txs); i++ {
		txs[i] = GetRandomTx()
		ISRs.RawRootsList[i] = GetRandomBytes(32)
	}
	ISRs.RawRootsList[len(txs)] = GetRandomBytes(32)

	txsWithISRs, err := txs.ToTxsWithISRs(ISRs)
	require.NoError(t, err)
	require.NotEmpty(t, txsWithISRs)

	txShares, err := TxsWithISRsToShares(txsWithISRs)
	require.NoError(t, err)
	require.NotEmpty(t, txShares)

	newTxsWithISRs, err := SharesToTxsWithISRs(txShares)
	require.NoError(t, err)
	require.NotEmpty(t, newTxsWithISRs)

	assert.Equal(t, txsWithISRs, newTxsWithISRs)

	for i := 0; i < 1000; i++ {
		numShares := rand.Int() % len(txShares)                    //nolint:gosec
		startShare := rand.Int() % (len(txShares) - numShares + 1) //nolint:gosec
		newTxsWithISRs, err := SharesToTxsWithISRs(txShares[startShare : startShare+numShares])
		require.NoError(t, err)
		isSubArray, err := checkSubArray(txsWithISRs, newTxsWithISRs)
		require.NoError(t, err)
		assert.True(t, isSubArray)
	}
}

// Returns whether subTxList is a subarray of txList
func checkSubArray(txList []rollkit.TxWithISRs, subTxList []rollkit.TxWithISRs) (bool, error) {
	for i := 0; i <= len(txList)-len(subTxList); i++ {
		j := 0
		for j = 0; j < len(subTxList); j++ {
			tx, err := txList[i+j].Marshal()
			if err != nil {
				return false, err
			}
			subTx, err := subTxList[j].Marshal()
			if err != nil {
				return false, err
			}
			if !bytes.Equal(tx, subTx) {
				break
			}
		}
		if j == len(subTxList) {
			return true, nil
		}
	}
	return false, nil
}
