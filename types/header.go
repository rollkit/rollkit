package types

import (
	"bytes"
	"encoding"
	"fmt"
	"time"

	"github.com/celestiaorg/go-header"
	tmbytes "github.com/tendermint/tendermint/libs/bytes"
)

type Hash = header.Hash

func ConvertToHexBytes(hash Hash) tmbytes.HexBytes {
	tmHash := make(tmbytes.HexBytes, 32)
	copy(tmHash, hash)
	return tmHash
}

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

	// Note that the address can be derived from the pubkey which can be derived
	// from the signature when using secp256k.
	// We keep this in case users choose another signature format where the
	// pubkey can't be recovered by the signature (e.g. ed25519).
	ProposerAddress []byte // original proposer of the block

	// Hash of block aggregator set, at a time of block creation
	AggregatorsHash Hash
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

func (h *Header) LastHeader() Hash {
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

func (h *Header) VerifyAdjacent(untrst header.Header) error {
	untrstH, ok := untrst.(*Header)
	if !ok {
		return &header.VerifyError{
			Reason: fmt.Errorf("%T is not of type %T", untrst, h),
		}
	}

	if untrstH.Height() != h.Height()+1 {
		return &header.VerifyError{
			Reason: fmt.Errorf("headers must be adjacent in height: trusted %d, untrusted %d", h.Height(), untrstH.Height()),
		}
	}

	if err := verifyNewHeaderAndVals(h, untrstH); err != nil {
		return &header.VerifyError{Reason: err}
	}

	// Check the validator hashes are the same
	// TODO: next validator set is not available
	if !bytes.Equal(untrstH.AggregatorsHash[:], h.AggregatorsHash[:]) {
		return &header.VerifyError{
			Reason: fmt.Errorf("expected old header next validators (%X) to match those from new header (%X)",
				h.AggregatorsHash,
				untrstH.AggregatorsHash,
			),
		}
	}

	return nil

}

func (h *Header) VerifyNonAdjacent(untrst header.Header) error {
	untrstH, ok := untrst.(*Header)
	if !ok {
		return &header.VerifyError{
			Reason: fmt.Errorf("%T is not of type %T", untrst, h),
		}
	}
	if untrstH.Height() == h.Height()+1 {
		return &header.VerifyError{
			Reason: fmt.Errorf(
				"headers must be non adjacent in height: trusted %d, untrusted %d",
				h.Height(),
				untrstH.Height(),
			),
		}
	}

	if err := verifyNewHeaderAndVals(h, untrstH); err != nil {
		return &header.VerifyError{Reason: err}
	}

	// Ensure that untrusted commit has enough of trusted commit's power.
	// err := h.ValidatorSet.VerifyCommitLightTrusting(eh.ChainID, untrst.Commit, light.DefaultTrustLevel)
	// if err != nil {
	// 	return &VerifyError{err}
	// }

	return nil
}

func (h *Header) Verify(h2 header.Header) error {
	// TODO(tzdybal): deprecated
	panic("implement me")
}

func (h *Header) Validate() error {
	return h.ValidateBasic()
}

// clockDrift defines how much new header's time can drift into
// the future relative to the now time during verification.
var maxClockDrift = 10 * time.Second

// verifyNewHeaderAndVals performs basic verification of untrusted header.
func verifyNewHeaderAndVals(trusted, untrusted *Header) error {
	if err := untrusted.ValidateBasic(); err != nil {
		return fmt.Errorf("untrusted.ValidateBasic failed: %w", err)
	}

	if untrusted.Height() <= trusted.Height() {
		return fmt.Errorf("expected new header height %d to be greater than one of old header %d",
			untrusted.Height(),
			trusted.Height())
	}

	if !untrusted.Time().After(trusted.Time()) {
		return fmt.Errorf("expected new header time %v to be after old header time %v",
			untrusted.Time(),
			trusted.Time())
	}

	if !untrusted.Time().Before(time.Now().Add(maxClockDrift)) {
		return fmt.Errorf("new header has a time from the future %v (now: %v; max clock drift: %v)",
			untrusted.Time(),
			time.Now(),
			maxClockDrift)
	}

	return nil
}

var _ header.Header = &Header{}
var _ encoding.BinaryMarshaler = &Header{}
var _ encoding.BinaryUnmarshaler = &Header{}
