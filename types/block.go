package types

import (
	"encoding"
	"fmt"
	"time"

	"github.com/celestiaorg/go-header"
	tmtypes "github.com/tendermint/tendermint/types"
)

type NamespaceID [8]byte

// Version captures the consensus rules for processing a block in the blockchain,
// including all blockchain data structures and the rules of the application's
// state transition machine.
// This is equivalent to the tmversion.Consensus type in Tendermint.
type Version struct {
	Block uint64
	App   uint64
}

// Block defines the structure of Rollkit block.
type Block struct {
	SignedHeader SignedHeader
	Data         Data
}

var _ encoding.BinaryMarshaler = &Block{}
var _ encoding.BinaryUnmarshaler = &Block{}

// Data defines Rollkit block data.
type Data struct {
	Txs                    Txs
	IntermediateStateRoots IntermediateStateRoots
	// Note: Temporarily remove Evidence #896
	// Evidence               EvidenceData
}

// EvidenceData defines how evidence is stored in block.
type EvidenceData struct {
	Evidence []Evidence
}

// Commit contains evidence of block creation.
type Commit struct {
	Signatures []Signature // most of the time this is a single signature
}

// SignedHeader combines Header and its Commit.
//
// Used mostly for gossiping.
type SignedHeader struct {
	Header
	Commit     Commit
	Validators *tmtypes.ValidatorSet
}

// Signature represents signature of block creator.
type Signature []byte

// IntermediateStateRoots describes the state between transactions.
// They are required for fraud proofs.
type IntermediateStateRoots struct {
	RawRootsList [][]byte
}

func (b *Block) New() header.Header {
	return new(Block)
}

func (b *Block) IsZero() bool {
	return b == nil
}

func (b *Block) ChainID() string {
	return b.SignedHeader.ChainID()
}

func (b *Block) Height() int64 {
	return b.SignedHeader.Height()
}

func (b *Block) LastHeader() Hash {
	return b.SignedHeader.LastHeader()
}

func (b *Block) Time() time.Time {
	return b.SignedHeader.Time()
}

func (b *Block) Verify(untrst header.Header) error {
	//TODO: Update with new header verify method
	untrstB, ok := untrst.(*Block)
	if !ok {
		// if the header type is wrong, something very bad is going on
		// and is a programmer bug
		panic(fmt.Errorf("%T is not of type %T", untrst, &b))
	}
	// sanity check fields
	if err := untrstB.ValidateBasic(); err != nil {
		return &header.VerifyError{Reason: err}
	}
	return nil
}

func (h *Block) Validate() error {
	return h.ValidateBasic()
}
