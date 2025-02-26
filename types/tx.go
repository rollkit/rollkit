package types

import (
	"fmt"

	"github.com/cometbft/cometbft/crypto/merkle"
	"github.com/cometbft/cometbft/crypto/tmhash"
	cmbytes "github.com/cometbft/cometbft/libs/bytes"
	"google.golang.org/protobuf/proto"

	"github.com/rollkit/rollkit/third_party/celestia-app/appconsts"
	appns "github.com/rollkit/rollkit/third_party/celestia-app/namespace"
	"github.com/rollkit/rollkit/third_party/celestia-app/shares"
	pb "github.com/rollkit/rollkit/types/pb/rollkit"
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
	txsWithISRs := make([]pb.TxWithISRs, len(txs))
	for i, tx := range txs {
		txsWithISRs[i] = pb.TxWithISRs{}
		txsWithISRs[i].SetPreIsr(intermediateStateRoots.RawRootsList[i])
		txsWithISRs[i].SetTx(tx)
		txsWithISRs[i].SetPostIsr(intermediateStateRoots.RawRootsList[i+1])
	}
	return txsWithISRs, nil
}

// TxsWithISRsToShares converts a slice of TxWithISRs to a slice of shares.
func TxsWithISRsToShares(txsWithISRs []pb.TxWithISRs) (txShares []shares.Share, err error) {
	byteSlices := make([][]byte, len(txsWithISRs))

	for i := 0; i < len(txsWithISRs); i++ {
		byteSlices[i], err = proto.Marshal(&txsWithISRs[i])
		if err != nil {
			return nil, err
		}
	}
	coreTxs := shares.TxsFromBytes(byteSlices)
	txShares, err = shares.SplitTxs(coreTxs)
	return txShares, err
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

// SharesToTxsWithISRs converts a slice of shares to a slice of TxWithISRs.
func SharesToTxsWithISRs(txShares []shares.Share) (txsWithISRs []*pb.TxWithISRs, err error) {
	byteSlices, err := shares.ParseCompactShares(txShares)
	if err != nil {
		return nil, err
	}
	txsWithISRs = make([]*pb.TxWithISRs, len(byteSlices))
	for i, byteSlice := range byteSlices {
		txWithISR := new(pb.TxWithISRs)
		err = proto.Unmarshal(byteSlice, txWithISR)
		if err != nil {
			return nil, err
		}
		txsWithISRs[i] = txWithISR
	}
	return txsWithISRs, nil
}
