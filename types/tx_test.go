package types

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	newTxsWithISRs, err := SharesToTxsWithISRs(newTxShares)
	require.NoError(err)
	require.NotEmpty(txsWithISRs)

	assert.Equal(txsWithISRs, newTxsWithISRs)
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
