package mock

import (
	"bytes"
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/celestiaorg/nmt"
	"github.com/celestiaorg/rsmt2d"

	"github.com/rollkit/celestia-openrpc/types/appconsts"
	"github.com/rollkit/celestia-openrpc/types/namespace"
	"github.com/rollkit/celestia-openrpc/types/share"
)

// Fulfills the rsmt2d.Tree interface and rsmt2d.TreeConstructorFn function
var (
	_ rsmt2d.TreeConstructorFn = NewConstructor(0)
	_ rsmt2d.Tree              = &ErasuredNamespacedMerkleTree{}
)

// ErasuredNamespacedMerkleTree wraps NamespaceMerkleTree to conform to the
// rsmt2d.Tree interface while also providing the correct namespaces to the
// underlying NamespaceMerkleTree. It does this by adding the already included
// namespace to the first half of the tree, and then uses the parity namespace
// ID for each share pushed to the second half of the tree. This allows for the
// namespaces to be included in the erasure data, while also keeping the nmt
// library sufficiently general
type ErasuredNamespacedMerkleTree struct {
	squareSize uint64 // note: this refers to the width of the original square before erasure-coded
	options    []nmt.Option
	tree       *nmt.NamespacedMerkleTree
	// axisIndex is the index of the axis (row or column) that this tree is on. This is passed
	// by rsmt2d and used to help determine which quadrant each leaf belongs to.
	axisIndex uint64
	// shareIndex is the index of the share in a row or column that is being
	// pushed to the tree. It is expected to be in the range: 0 <= shareIndex <
	// 2*squareSize. shareIndex is used to help determine which quadrant each
	// leaf belongs to, along with keeping track of how many leaves have been
	// added to the tree so far.
	shareIndex uint64
}

// NewErasuredNamespacedMerkleTree creates a new ErasuredNamespacedMerkleTree
// with an underlying NMT of namespace size `namespace.NamespaceSize` and with
// `ignoreMaxNamespace=true`. axisIndex is the index of the row or column that
// this tree is committing to. squareSize must be greater than zero.
func NewErasuredNamespacedMerkleTree(squareSize uint64, axisIndex uint, options ...nmt.Option) ErasuredNamespacedMerkleTree {
	if squareSize == 0 {
		panic("cannot create a ErasuredNamespacedMerkleTree of squareSize == 0")
	}
	options = append(options, nmt.NamespaceIDSize(namespace.NamespaceSize))
	options = append(options, nmt.IgnoreMaxNamespace(true))
	tree := nmt.New(namespace.NewBaseHashFunc(), options...)
	return ErasuredNamespacedMerkleTree{squareSize: squareSize, options: options, tree: tree, axisIndex: uint64(axisIndex), shareIndex: 0}
}

type constructor struct {
	squareSize uint64
	opts       []nmt.Option
}

// NewConstructor creates a tree constructor function as required by rsmt2d to
// calculate the data root. It creates that tree using the
// wrapper.ErasuredNamespacedMerkleTree.
func NewConstructor(squareSize uint64, opts ...nmt.Option) rsmt2d.TreeConstructorFn {
	return constructor{
		squareSize: squareSize,
		opts:       opts,
	}.NewTree
}

// NewTree creates a new rsmt2d.Tree using the
// wrapper.ErasuredNamespacedMerkleTree with predefined square size and
// nmt.Options
func (c constructor) NewTree(_ rsmt2d.Axis, axisIndex uint) rsmt2d.Tree {
	newTree := NewErasuredNamespacedMerkleTree(c.squareSize, axisIndex, c.opts...)
	return &newTree
}

// Push adds the provided data to the underlying NamespaceMerkleTree, and
// automatically uses the first DefaultNamespaceIDLen number of bytes as the
// namespace unless the data pushed to the second half of the tree. Fulfills the
// rsmt.Tree interface. NOTE: panics if an error is encountered while pushing or
// if the tree size is exceeded.
func (w *ErasuredNamespacedMerkleTree) Push(data []byte) error {
	if w.axisIndex+1 > 2*w.squareSize || w.shareIndex+1 > 2*w.squareSize {
		return fmt.Errorf("pushed past predetermined square size: boundary at %d index at %d %d", 2*w.squareSize, w.axisIndex, w.shareIndex)
	}
	if len(data) < namespace.NamespaceSize {
		return fmt.Errorf("data is too short to contain namespace ID")
	}
	nidAndData := make([]byte, namespace.NamespaceSize+len(data))
	copy(nidAndData[namespace.NamespaceSize:], data)
	// use the parity namespace if the cell is not in Q0 of the extended data square
	if w.isQuadrantZero() {
		copy(nidAndData[:namespace.NamespaceSize], data[:namespace.NamespaceSize])
	} else {
		copy(nidAndData[:namespace.NamespaceSize], namespace.ParitySharesNamespace.Bytes())
	}
	err := w.tree.Push(nidAndData)
	if err != nil {
		return err
	}
	w.incrementShareIndex()
	return nil
}

// Root fulfills the rsmt.Tree interface by generating and returning the
// underlying NamespaceMerkleTree Root.
func (w *ErasuredNamespacedMerkleTree) Root() ([]byte, error) {
	root, err := w.tree.Root()
	if err != nil {
		return nil, err
	}
	return root, nil
}

// ProveRange returns a Merkle range proof for the leaf range [start, end] where `end` is non-inclusive.
func (w *ErasuredNamespacedMerkleTree) ProveRange(start, end int) (nmt.Proof, error) {
	return w.tree.ProveRange(start, end)
}

// incrementShareIndex increments the share index by one.
func (w *ErasuredNamespacedMerkleTree) incrementShareIndex() {
	w.shareIndex++
}

// isQuadrantZero returns true if the current share index and axis index are both
// in the original data square.
func (w *ErasuredNamespacedMerkleTree) isQuadrantZero() bool {
	return w.shareIndex < w.squareSize && w.axisIndex < w.squareSize
}

// RandEDS generates EDS filled with the random data with the given size for original square. It
// uses require.TestingT to be able to take both a *testing.T and a *testing.B.
func RandEDS(size int) (*rsmt2d.ExtendedDataSquare, error) {
	shares, err := RandShares(size * size)
	if err != nil {
		return nil, err
	}
	// recompute the eds
	return rsmt2d.ComputeExtendedDataSquare(shares, share.DefaultRSMT2DCodec(), NewConstructor(uint64(size)))
}

// RandShares generate 'total' amount of shares filled with random data. It uses require.TestingT
// to be able to take both a *testing.T and a *testing.B.
func RandShares(total int) ([]share.Share, error) {
	if total&(total-1) != 0 {
		return nil, fmt.Errorf("total must be power of 2: %d", total)
	}

	var r = rand.New(rand.NewSource(time.Now().Unix())) //nolint:gosec
	shares := make([]share.Share, total)
	for i := range shares {
		shr := make([]byte, appconsts.ShareSize)
		copy(shr[:appconsts.NamespaceSize], RandNamespace())
		_, err := r.Read(shr[appconsts.NamespaceSize:])
		if err != nil {
			return nil, err
		}
		shares[i] = shr
	}
	sort.Slice(shares, func(i, j int) bool { return bytes.Compare(shares[i], shares[j]) < 0 })

	return shares, nil
}

// RandNamespace generates random valid data namespace for testing purposes.
func RandNamespace() share.Namespace {
	var r = rand.New(rand.NewSource(time.Now().Unix())) //nolint:gosec
	rb := make([]byte, namespace.NamespaceVersionZeroIDSize)
	r.Read(rb) // nolint:gosec
	for {
		namespace, _ := share.NewBlobNamespaceV0(rb)
		if err := namespace.ValidateForData(); err != nil {
			continue
		}
		return namespace
	}
}
