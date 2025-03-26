package types

import (
	"bytes"
	"encoding"
	"errors"
	"fmt"
	"time"

	"github.com/celestiaorg/go-header"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/rollkit/rollkit/types/pb/rollkit/v1"
)

// Hash is a 32-byte array which is used to represent a hash result.
type Hash = header.Hash

var (
	// ErrNoProposerAddress is returned when the proposer address is not set.
	ErrNoProposerAddress = errors.New("no proposer address")

	// ErrProposerVerificationFailed is returned when the proposer verification fails.
	ErrProposerVerificationFailed = errors.New("proposer verification failed")
)

// BaseHeader contains the most basic data of a header
type BaseHeader struct {
	// Height represents the block height (aka block number) of a given header
	Height uint64
	// Time contains Unix nanotime of a block
	Time uint64
	// The Chain ID
	ChainID string
}

// Header defines the structure of Rollkit block header.
type Header struct {
	BaseHeader
	// Block and App version
	Version Version

	// prev block info
	LastHeaderHash Hash

	// hashes of block data
	LastCommitHash Hash // commit from aggregator(s) from the last block
	DataHash       Hash // Block.Data root aka Transactions
	ConsensusHash  Hash // consensus params for current block
	AppHash        Hash // state after applying txs from the current block

	// Root hash of all results from the txs from the previous block.
	// This is ABCI specific but smart-contract chains require some way of committing
	// to transaction receipts/results.
	LastResultsHash Hash

	// compatibility with tendermint light client
	ValidatorHash Hash

	// Note that the address can be derived from the pubkey which can be derived
	// from the signature when using secp256k.
	// We keep this in case users choose another signature format where the
	// pubkey can't be recovered by the signature (e.g. ed25519).
	ProposerAddress []byte // original proposer of the block
}

// New creates a new Header.
func (h *Header) New() *Header {
	return new(Header)
}

// IsZero returns true if the header is nil.
func (h *Header) IsZero() bool {
	return h == nil
}

// ChainID returns chain ID of the header.
func (h *Header) ChainID() string {
	return h.BaseHeader.ChainID
}

// Height returns height of the header.
func (h *Header) Height() uint64 {
	return h.BaseHeader.Height
}

// LastHeader returns last header hash of the header.
func (h *Header) LastHeader() Hash {
	return h.LastHeaderHash[:]
}

// Time returns timestamp as unix time with nanosecond precision
func (h *Header) Time() time.Time {
	return time.Unix(0, int64(h.BaseHeader.Time)) //nolint:gosec
}

// Verify verifies the header.
func (h *Header) Verify(untrstH *Header) error {
	if !bytes.Equal(untrstH.ProposerAddress, h.ProposerAddress) {
		return &header.VerifyError{
			Reason: fmt.Errorf("%w: expected proposer (%X) got (%X)",
				ErrProposerVerificationFailed,
				h.ProposerAddress,
				untrstH.ProposerAddress,
			),
		}
	}
	return nil
}

// Validate performs basic validation of a header.
func (h *Header) Validate() error {
	return h.ValidateBasic()
}

// ValidateBasic performs basic validation of a header.
func (h *Header) ValidateBasic() error {
	if len(h.ProposerAddress) == 0 {
		return ErrNoProposerAddress
	}

	return nil
}

// Vote returns a vote for the header.
func (h *Header) Vote() ([]byte, error) {
	v := &v1.Vote{
		ChainId:          h.ChainID(),
		Height:           h.Height(),
		BlockIdHash:      h.Hash(),
		ValidatorAddress: h.ProposerAddress,
		Timestamp:        timestamppb.New(h.Time()),
	}

	bz, err := proto.Marshal(v)
	if err != nil {
		return nil, err
	}
	return bz, nil
}

var _ header.Header[*Header] = &Header{}
var _ encoding.BinaryMarshaler = &Header{}
var _ encoding.BinaryUnmarshaler = &Header{}
