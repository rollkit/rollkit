package testfactory

import (
	"bytes"
	"math/rand"
	mrand "math/rand"

	"github.com/cometbft/cometbft/types"
)

func GenerateRandomlySizedTxs(count, maxSize int) types.Txs {
	txs := make(types.Txs, count)
	for i := 0; i < count; i++ {
		size := mrand.Intn(maxSize) //nolint:gosec
		if size == 0 {
			size = 1
		}
		txs[i] = GenerateRandomTxs(1, size)[0]
	}
	return txs
}

func GenerateRandomTxs(count, size int) types.Txs {
	txs := make(types.Txs, count)
	for i := 0; i < count; i++ {
		tx := make([]byte, size)
		_, err := mrand.Read(tx) //nolint:gosec,staticcheck
		if err != nil {
			panic(err)
		}
		txs[i] = tx
	}
	return txs
}

// GetRandomSubSlice returns two integers representing a randomly sized range in the interval [0, size]
func GetRandomSubSlice(size int) (start int, length int) {
	length = rand.Intn(size + 1)         //nolint:gosec
	start = rand.Intn(size - length + 1) //nolint:gosec
	return start, length
}

// CheckSubArray returns whether subTxList is a subarray of txList
func CheckSubArray(txList []types.Tx, subTxList []types.Tx) bool {
	for i := 0; i <= len(txList)-len(subTxList); i++ {
		j := 0
		for j = 0; j < len(subTxList); j++ {
			tx := txList[i+j]
			subTx := subTxList[j]
			if !bytes.Equal([]byte(tx), []byte(subTx)) {
				break
			}
		}
		if j == len(subTxList) {
			return true
		}
	}
	return false
}
