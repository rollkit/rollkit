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
	require := require.New(t)
	assert := assert.New(t)

	txs := make(Txs, 1000)
	ISRs := IntermediateStateRoots{
		RawRootsList: make([][]byte, len(txs)+1),
	}
	for i := 0; i < len(txs); i++ {
		txs[i] = getRandomTx()
		ISRs.RawRootsList[i] = getRandomBytes(32)
	}
	ISRs.RawRootsList[len(txs)] = getRandomBytes(32)

	txsWithISRs, err := txs.ToTxsWithISRs(ISRs)
	require.NoError(err)
	require.NotEmpty(txsWithISRs)

	txShares, err := TxsWithISRsToShares(txsWithISRs)
	require.NoError(err)
	require.NotEmpty(txShares)

	txBytes, err := SharesToPostableBytes(txShares)
	require.NoError(err)
	require.NotEmpty(txBytes)

	newTxShares, err := PostableBytesToShares(txBytes)
	require.NoError(err)
	require.NotEmpty(newTxShares)

	// Note that txShares and newTxShares are not necessarily equal because newTxShares might
	// contain zero padding at the end and thus sequence length can differ
	newTxsWithISRs, err := SharesToTxsWithISRs(newTxShares)
	require.NoError(err)
	require.NotEmpty(txsWithISRs)

	assert.Equal(txsWithISRs, newTxsWithISRs)
}

func TestTxWithISRSerializationOutOfContextRoundtrip(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	numTxs := 1000
	txs := make(Txs, numTxs)
	ISRs := IntermediateStateRoots{
		RawRootsList: make([][]byte, len(txs)+1),
	}
	for i := 0; i < len(txs); i++ {
		txs[i] = getRandomTx()
		ISRs.RawRootsList[i] = getRandomBytes(32)
	}
	ISRs.RawRootsList[len(txs)] = getRandomBytes(32)

	txsWithISRs, err := txs.ToTxsWithISRs(ISRs)
	require.NoError(err)
	require.NotEmpty(txsWithISRs)

	txShares, err := TxsWithISRsToShares(txsWithISRs)
	require.NoError(err)
	require.NotEmpty(txShares)

	newTxsWithISRs, err := SharesToTxsWithISRs(txShares)
	require.NoError(err)
	require.NotEmpty(newTxsWithISRs)

	assert.Equal(txsWithISRs, newTxsWithISRs)

	for i := 0; i < 1000; i++ {
		numShares := rand.Int() % len(txShares)                    //nolint: gosec
		startShare := rand.Int() % (len(txShares) - numShares + 1) //nolint: gosec
		newTxsWithISRs, err := SharesToTxsWithISRs(txShares[startShare : startShare+numShares])
		require.NoError(err)
		assert.True(checkSubArray(txsWithISRs, newTxsWithISRs))
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

// copy-pasted from store/store_test.go
func getRandomTx() Tx {
	size := rand.Int()%100 + 100 //nolint:gosec
	return Tx(getRandomBytes(size))
}

// copy-pasted from store/store_test.go
func getRandomBytes(n int) []byte {
	data := make([]byte, n)
	_, _ = rand.Read(data) //nolint:gosec
	return data
}
