package types

import (
	"context"
	"encoding"
	"errors"
	"fmt"
	"time"

	"github.com/celestiaorg/go-header"
)

type headerContextKey struct{}

// HeaderContextKey is used to store the header in the context.
// This is useful if the execution client needs to access the header during transaction execution.
var HeaderContextKey = headerContextKey{}

func HeaderFromContext(ctx context.Context) (Header, bool) {
	h, ok := ctx.Value(HeaderContextKey).(Header)
	if !ok {
		return Header{}, false
	}

	// Clear the memoized hash on the returned copy. The caller may mutate
	// fields and call Hash() without knowing to call InvalidateHash() first,
	// which would return a stale cached value.
	h.InvalidateHash()
	return h, true
}

// Hash is a 32-byte array which is used to represent a hash result.
type Hash = header.Hash

const (
	// MaxClockDrift is the maximum allowed clock drift for timestamp validation.
	// This allows for slight time differences between nodes in a distributed system.
	MaxClockDrift = 1 * time.Minute
)

var (
	// ErrNoProposerAddress is returned when the proposer address is not set.
	ErrNoProposerAddress = errors.New("no proposer address")

	// ErrInvalidTimestamp is returned when the timestamp is invalid.
	ErrInvalidTimestamp = errors.New("invalid timestamp")
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

// Header defines the structure of an Evolve block header.
type Header struct {
	BaseHeader
	// Block and App version
	Version Version

	// prev block info
	LastHeaderHash Hash

	// hashes of block data
	DataHash Hash // Block.Data root aka Transactions
	AppHash  Hash // state after applying txs from the current block

	// compatibility with tendermint light client
	ValidatorHash Hash

	// Note that the address can be derived from the pubkey which can be derived
	// from the signature when using secp256k.
	// We keep this in case users choose another signature format where the
	// pubkey can't be recovered by the signature (e.g. ed25519).
	ProposerAddress []byte // original proposer of the block

	// Legacy holds fields that were removed from the canonical header JSON/Go
	// representation but may still be required for backwards compatible binary
	// serialization (e.g. legacy signing payloads).
	Legacy *LegacyHeaderFields

	// cachedHash holds the memoized result of MemoizeHash(). nil means cold.
	// Any caller that mutates header fields must call InvalidateHash() to clear it.
	cachedHash Hash
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

// LastHeader returns the last header hash.
func (h *Header) LastHeader() Hash {
	return h.LastHeaderHash[:]
}

// Time returns timestamp as unix time with nanosecond precision
func (h *Header) Time() time.Time {
	return time.Unix(0, int64(h.BaseHeader.Time))
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

	// Validate timestamp - must be non-zero and not too far in the future
	if h.BaseHeader.Time == 0 {
		return fmt.Errorf("%w: timestamp cannot be zero", ErrInvalidTimestamp)
	}

	// Check timestamp is not too far in the future (allow some clock drift)
	maxAllowedTime := time.Now().Add(MaxClockDrift)
	headerTime := h.Time()
	if headerTime.After(maxAllowedTime) {
		return fmt.Errorf("%w: timestamp too far in future (header: %v, max allowed: %v)",
			ErrInvalidTimestamp, headerTime, maxAllowedTime)
	}

	return nil
}

var (
	_ encoding.BinaryMarshaler   = &Header{}
	_ encoding.BinaryUnmarshaler = &Header{}
)

// LegacyHeaderFields captures the deprecated header fields that existed prior
// to the header minimisation change. When populated, these values are reused
// while constructing the protobuf payload so that legacy nodes can continue to
// verify signatures and hashes.
//
// # Migration Guide
//
// This compatibility layer enables networks to sync from genesis after the header
// minimization changes. The system automatically handles both legacy and slim
// header formats through a multi-format verification fallback mechanism.
//
// ## Format Detection
//
// Headers are decoded and verified using the following strategy:
//  1. Try custom signature provider (if configured)
//  2. Try slim header format (new format without legacy fields)
//  3. Try legacy header format (includes LastCommitHash, ConsensusHash, LastResultsHash)
//
// The Legacy field is automatically populated during deserialization when legacy
// fields are detected in the protobuf unknown fields (field numbers 5, 7, 9).
//
// ## For Block Producers
//
// New blocks should use the slim header format by default (Legacy == nil).
// The legacy encoding is only required when:
//   - Syncing historical blocks from genesis
//   - Interoperating with legacy nodes
//   - Verifying signatures on historical blocks
//
// ## For Node Operators
//
// Nodes will automatically:
//   - Decode legacy headers when syncing from genesis
//   - Verify signatures using the appropriate format
//   - Handle mixed networks with both old and new nodes
//
// No configuration changes are required for the migration.
//
// ## Legacy Field Defaults
//
// When encoding legacy headers, ConsensusHash defaults to a 32-byte zero hash
// if not explicitly set, matching the historical behavior. Other legacy fields
// remain nil if unset.
type LegacyHeaderFields struct {
	LastCommitHash  Hash
	ConsensusHash   Hash
	LastResultsHash Hash
}

// IsZero reports whether all legacy fields are unset.
func (l *LegacyHeaderFields) IsZero() bool {
	if l == nil {
		return true
	}
	return len(l.LastCommitHash) == 0 &&
		len(l.ConsensusHash) == 0 &&
		len(l.LastResultsHash) == 0
}

// EnsureDefaults initialises missing legacy fields with their historical
// default values so that the legacy protobuf payload matches the pre-change
// encoding.
func (l *LegacyHeaderFields) EnsureDefaults() {
	if l.ConsensusHash == nil {
		l.ConsensusHash = make(Hash, 32)
	}
}

// Clone returns a deep copy of the legacy fields.
func (l *LegacyHeaderFields) Clone() *LegacyHeaderFields {
	if l == nil {
		return nil
	}
	clone := &LegacyHeaderFields{
		LastCommitHash:  cloneBytes(l.LastCommitHash),
		ConsensusHash:   cloneBytes(l.ConsensusHash),
		LastResultsHash: cloneBytes(l.LastResultsHash),
	}
	return clone
}

// ApplyLegacyDefaults ensures the Header has a Legacy block initialised with
// the expected defaults so that legacy serialization works without callers
// needing to populate every field explicitly.
func (h *Header) ApplyLegacyDefaults() {
	if h.Legacy == nil {
		h.Legacy = &LegacyHeaderFields{}
	}
	h.Legacy.EnsureDefaults()
	h.InvalidateHash()
}

// Clone creates a deep copy of the header, ensuring all mutable slices are
// duplicated to avoid unintended sharing between variants.
func (h Header) Clone() Header {
	clone := h
	clone.LastHeaderHash = cloneBytes(h.LastHeaderHash)
	clone.DataHash = cloneBytes(h.DataHash)
	clone.AppHash = cloneBytes(h.AppHash)
	clone.ValidatorHash = cloneBytes(h.ValidatorHash)
	clone.ProposerAddress = cloneBytes(h.ProposerAddress)
	clone.Legacy = h.Legacy.Clone()
	clone.cachedHash = nil

	return clone
}

func cloneBytes(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return append([]byte(nil), b...)
}
