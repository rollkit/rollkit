package types

import (
	"fmt"

	"github.com/cometbft/cometbft/crypto/merkle"
	"github.com/cometbft/cometbft/crypto/tmhash"
	cmbytes "github.com/cometbft/cometbft/libs/bytes"

	pb "github.com/rollkit/rollkit/types/pb/rollkit"
)

// Tx represents transactoin.
type Tx []byte

// Txs represents a slice of transactions.
type Txs []Tx

// Hash computes the TMHASH hash of the wire encoded transaction.
func (tx Tx) Hash() []byte {
	return tmhash.Sum(tx)
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

// ToTxsWithISRs converts a slice of transactions and a list of intermediate state roots
// to a slice of TxWithISRs. Note that the length of intermediateStateRoots is
// equal to the length of txs + 1.
func (txs Txs) ToTxsWithISRs(intermediateStateRoots IntermediateStateRoots) ([]pb.TxWithISRs, error) {
	expectedISRListLength := len(txs) + 1
	if len(intermediateStateRoots.RawRootsList) != expectedISRListLength {
		return nil, fmt.Errorf("invalid length of ISR list: %d, expected length: %d", len(intermediateStateRoots.RawRootsList), expectedISRListLength)
	}
	txsWithISRs := make([]pb.TxWithISRs, 0, len(txs))
	for i, tx := range txs {
		txsWithISRs[i] = pb.TxWithISRs{
			PreIsr:  intermediateStateRoots.RawRootsList[i],
			Tx:      tx,
			PostIsr: intermediateStateRoots.RawRootsList[i+1],
		}
	}
	return txsWithISRs, nil
}
