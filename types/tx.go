package types

import (
	"github.com/cometbft/cometbft/crypto/merkle"
	cmhash "github.com/cometbft/cometbft/crypto/tmhash"
	cmbytes "github.com/cometbft/cometbft/libs/bytes"
)

// Tx represents transactoin.
type Tx []byte

// Txs represents a slice of transactions.
type Txs []Tx

// Hash computes the TMHASH hash of the wire encoded transaction.
func (tx Tx) Hash() []byte {
	return cmhash.Sum(tx)
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
	RootHash cmbytes.HexBytes `json:"root_hash"`
	Data     Tx               `json:"data"`
	Proof    merkle.Proof     `json:"proof"`
}
