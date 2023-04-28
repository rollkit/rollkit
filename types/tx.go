package types

import (
	"fmt"

	"github.com/tendermint/tendermint/crypto/merkle"
	"github.com/tendermint/tendermint/crypto/tmhash"
	tmbytes "github.com/tendermint/tendermint/libs/bytes"

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
	RootHash tmbytes.HexBytes `json:"root_hash"`
	Data     Tx               `json:"data"`
	Proof    merkle.Proof     `json:"proof"`
}

// ToTxsWithISRs converts a slice of transactions and a list of intermediate state roots
// to a slice of TxWithISRs. It assumes that the length of intermediateStateRoots is
// equal to the length of txs + 2.
func (txs Txs) ToTxsWithISRs(intermediateStateRoots IntermediateStateRoots) ([]pb.TxWithISRs, error) {
	expectedISRLength := len(txs) + 2
	if len(intermediateStateRoots.RawRootsList) != expectedISRLength {
		return nil, fmt.Errorf("invalid length of ISR list: %d, expected length: %d", len(intermediateStateRoots.RawRootsList), expectedISRLength)
	}
	size := len(txs)
	txsWithISRs := make([]pb.TxWithISRs, 0, size)
	for i, tx := range txs {
		txsWithISRs[i] = pb.TxWithISRs{
			PreIsr:  intermediateStateRoots.RawRootsList[i],
			Tx:      tx,
			PostIsr: intermediateStateRoots.RawRootsList[i+1],
		}
	}
	return txsWithISRs, nil
}
