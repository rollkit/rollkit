package types

import (
	"encoding"
	"time"

	"github.com/celestiaorg/go-header"
)

// BaseHeader contains the most basic data of a header
type BaseHeader struct {
	// Height represents the block height (aka block number) of a given header
	Height uint64
	// Time contains Unix time of a block
	Time uint64
	// The Chain ID
	ChainID string
}

// Header defines the structure of rollmint block header.
type Header struct {
	BaseHeader
	// Block and App version
	Version Version
	// NamespaceID identifies this chain e.g. when connected to other rollups via IBC.
	// TODO(ismail): figure out if we want to use namespace.ID here instead (downside is that it isn't fixed size)
	// at least extract the used constants (32, 8) as package variables though.
	NamespaceID NamespaceID

	// prev block info
	LastHeaderHash [32]byte

	// hashes of block data
	LastCommitHash [32]byte // commit from aggregator(s) from the last block
	DataHash       [32]byte // Block.Data root aka Transactions
	ConsensusHash  [32]byte // consensus params for current block
	AppHash        [32]byte // state after applying txs from the current block

	// Root hash of all results from the txs from the previous block.
	// This is ABCI specific but smart-contract chains require some way of committing
	// to transaction receipts/results.
	LastResultsHash [32]byte

	// Note that the address can be derived from the pubkey which can be derived
	// from the signature when using secp256k.
	// We keep this in case users choose another signature format where the
	// pubkey can't be recovered by the signature (e.g. ed25519).
	ProposerAddress []byte // original proposer of the block

	// Hash of block aggregator set, at a time of block creation
	AggregatorsHash [32]byte
}

func (h *Header) New() header.Header {
	return new(Header)
}

func (h *Header) ChainID() string {
	return h.BaseHeader.ChainID
}

func (h *Header) Height() int64 {
	return int64(h.BaseHeader.Height)
}

func (h *Header) LastHeader() header.Hash {
	return h.LastHeaderHash[:]
}

func (h *Header) Time() time.Time {
	return time.Unix(int64(h.BaseHeader.Time), 0)
}

func (h *Header) IsRecent(blockTime time.Duration) bool {
	return time.Since(h.Time()) <= blockTime
}

func (h *Header) IsExpired() bool {
	// TODO(tzdybal): TrustingPeriod will be configurable soon (https://github.com/celestiaorg/celestia-node/pull/1544)
	const TrustingPeriod = 168 * time.Hour
	expirationTime := h.Time().Add(TrustingPeriod)
	return !expirationTime.After(time.Now())
}

func (h *Header) VerifyAdjacent(h2 header.Header) error {
	//TODO implement me
	panic("implement me")
}

func (h *Header) VerifyNonAdjacent(h2 header.Header) error {
	//TODO implement me
	panic("implement me")
}

func (h *Header) Verify(h2 header.Header) error {
	// TODO(tzdybal): deprecated
	panic("implement me")
}

func (h *Header) Validate() error {
	return h.ValidateBasic()
}

var _ header.Header = &Header{}
var _ encoding.BinaryMarshaler = &Header{}
var _ encoding.BinaryUnmarshaler = &Header{}
