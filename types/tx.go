package types

import (
	"github.com/cometbft/cometbft/crypto/merkle"
	"github.com/cometbft/cometbft/crypto/tmhash"

	"github.com/rollkit/rollkit/third_party/celestia-app/appconsts"
	appns "github.com/rollkit/rollkit/third_party/celestia-app/namespace"
	"github.com/rollkit/rollkit/third_party/celestia-app/shares"
)

// Tx represents transaction.
type Tx []byte

// Txs represents a slice of transactions.
type Txs []Tx

// Hash computes the TMHASH hash of the wire encoded transaction.
func (tx Tx) Hash() []byte {
	return tmhash.Sum(tx)
}

// ToSliceOfBytes converts a Txs to slice of byte slices.
func (txs Txs) ToSliceOfBytes() [][]byte {
	txBzs := make([][]byte, len(txs))
	for i := 0; i < len(txs); i++ {
		txBzs[i] = txs[i]
	}
	return txBzs
}

// Proof returns a simple merkle proof for this node.
// Panics if i < 0 or i >= len(txs)
// TODO: optimize this!
func (txs Txs) Proof(i int) TxProof {
	l := len(txs)
	bzs := make([][]byte, l)
	for i := 0; i < l; i++ {
		bzs[i] = txs[i].Hash()
	}
	root, proofs := merkle.ProofsFromByteSlices(bzs)

	return TxProof{
		RootHash: root,
		Data:     txs[i],
		Proof:    *proofs[i],
	}
}

// TxProof represents a Merkle proof of the presence of a transaction in the Merkle tree.
type TxProof struct {
	RootHash []byte       `json:"root_hash"`
	Data     Tx           `json:"data"`
	Proof    merkle.Proof `json:"proof"`
}

// SharesToPostableBytes converts a slice of shares to a slice of bytes that can
// be posted to the blockchain.
func SharesToPostableBytes(txShares []shares.Share) (postableData []byte, err error) {
	for i := 0; i < len(txShares); i++ {
		raw, err := txShares[i].RawDataWithReserved()
		if err != nil {
			return nil, err
		}
		postableData = append(postableData, raw...)
	}
	return postableData, nil
}

// PostableBytesToShares converts a slice of bytes that can be posted to the
// blockchain to a slice of shares
func PostableBytesToShares(postableData []byte) (txShares []shares.Share, err error) {
	css := shares.NewCompactShareSplitterWithIsCompactFalse(appns.TxNamespace, appconsts.ShareVersionZero)
	err = css.WriteWithNoReservedBytes(postableData)
	if err != nil {
		return nil, err
	}
	shares, _, err := css.Export(0)
	return shares, err
}
