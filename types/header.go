package types

import (
	"bytes"
	"encoding"
	"fmt"
	"time"
)

// Header abstracts all methods required to perform header sync.
type H interface {
	// New creates new instance of a header.
	New() Header
	// Hash returns hash of a header.
	Hash() [32]byte
	// Height returns the height of a header.
	// Height() int64
	// LastHeader returns the hash of last header before this header (aka. previous header hash).
	LastHeader() [32]byte
	// Time returns time when header was created.
	// Time() time.Time
	// IsRecent checks if header is recent against the given blockTime.
	IsRecent(duration time.Duration) bool
	// IsExpired checks if header is expired against trusting period.
	IsExpired() bool
	// VerifyAdjacent validates adjacent untrusted header against trusted header.
	VerifyAdjacent(Header) error
	// VerifyNonAdjacent validates non-adjacent untrusted header against trusted header.
	VerifyNonAdjacent(Header) error
	// Verify performs basic verification of untrusted header.
	Verify(Header) error
	// Validate performs basic validation to check for missed/incorrect fields.
	Validate() error

	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

var _ H = (*Header)(nil)

// New creates new instance of a header.
func (h *Header) New() Header {
	return Header{}
}

// LastHeader returns the hash of last header before this header (aka. previous header hash).
func (h *Header) LastHeader() [32]byte {
	return h.LastHeaderHash
}

// IsRecent checks if header is recent against the given blockTime.
func (h *Header) IsRecent(blockTime time.Duration) bool {
	return time.Since(time.Unix(0, int64(h.Time))) <= blockTime
}

// TODO(@Wondertan): We should request TrustingPeriod from the network's state params or
//  listen for network params changes to always have a topical value.

// TrustingPeriod is period through which we can trust a header's validators set.
//
// Should be significantly less than the unbonding period (e.g. unbonding
// period = 3 weeks, trusting period = 2 weeks).
//
// More specifically, trusting period + time needed to check headers + time
// needed to report and punish misbehavior should be less than the unbonding
// period.
var TrustingPeriod = 168 * time.Hour

// IsExpired checks if header is expired against trusting period.
func (h *Header) IsExpired() bool {
	expirationTime := time.Unix(0, int64(h.Time)).Add(TrustingPeriod)
	return !expirationTime.After(time.Now())
}

// VerifyError is thrown on during VerifyAdjacent and VerifyNonAdjacent if verification fails.
type VerifyError struct {
	Reason error
}

func (vr *VerifyError) Error() string {
	return fmt.Sprintf("header: verify: %s", vr.Reason.Error())
}

// ErrNonAdjacent is returned when Store is appended with a header not adjacent to the stored head.
type ErrNonAdjacent struct {
	Head      uint64
	Attempted uint64
}

func (ena *ErrNonAdjacent) Error() string {
	return fmt.Sprintf("header/store: non-adjacent: head %d, attempted %d", ena.Head, ena.Attempted)
}

// VerifyAdjacent validates adjacent untrusted header against trusted header.
func (h *Header) VerifyAdjacent(untrst Header) error {
	if untrst.Height != h.Height+1 {
		return &ErrNonAdjacent{
			Head:      h.Height,
			Attempted: untrst.Height,
		}
	}

	if err := h.Verify(untrst); err != nil {
		return &VerifyError{Reason: err}
	}

	// Check the validator hashes are the same
	if !bytes.Equal(untrst.AggregatorsHash[:], h.AggregatorsHash[:]) {
		return &VerifyError{
			fmt.Errorf("expected old header next validators (%X) to match those from new header (%X)",
				h.AggregatorsHash,
				untrst.AggregatorsHash,
			),
		}
	}

	return nil
}

// VerifyNonAdjacent validates non-adjacent untrusted header against trusted header.
func (h *Header) VerifyNonAdjacent(untrst Header) error {
	if err := h.Verify(untrst); err != nil {
		return &VerifyError{Reason: err}
	}

	// Ensure that untrusted commit has enough of trusted commit's power.
	// err := h.ValidatorSet.VerifyCommitLightTrusting(eh.ChainID, untrst.Commit, light.DefaultTrustLevel)
	// if err != nil {
	// 	return &VerifyError{err}
	// }

	return nil
}

// clockDrift defines how much new header's time can drift into
// the future relative to the now time during verification.
var clockDrift = 10 * time.Second

// Verify performs basic verification of untrusted header.
func (h *Header) Verify(untrst Header) error {
	untrstTime := time.Unix(0, int64(untrst.Time))
	hTime := time.Unix(0, int64(h.Time))
	if !untrstTime.After(hTime) {
		return fmt.Errorf("expected new untrusted header time %v to be after old header time %v", untrstTime, hTime)
	}

	now := time.Now()
	if !untrstTime.Before(now.Add(clockDrift)) {
		return fmt.Errorf(
			"new untrusted header has a time from the future %v (now: %v, clockDrift: %v)", untrstTime, now, clockDrift)
	}

	return nil
}

// Validate performs basic validation to check for missed/incorrect fields.
func (h *Header) Validate() error {
	return h.ValidateBasic()
}
